package client

import (
	"context"
	"crypto/sha1"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	capn "github.com/glycerine/go-capnproto"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/xlab/beam-o/proto"
	"github.com/xlab/pace"
	kcp "github.com/xtaci/kcp-go"
	"github.com/xtaci/smux"
	"golang.org/x/crypto/pbkdf2"
)

type Client interface {
	StartBeam(ctx context.Context, totalCount int64, files <-chan string)
	GetProcessed() int64
	Config() *Config
}

type kcpClient struct {
	cfg         *Config
	syncRoot    string
	syncRemote  string
	processed   int64
	beamPace    pace.Pace
	sessions    []sessionWithTTL
	sessionsMux *sync.RWMutex
	chScavenger chan *smux.Session
	smuxConfig  *smux.Config
	blockCrypt  kcp.BlockCrypt
}

func NewKCP(cfg *Config, syncRoot, syncRemote string) Client {
	s := &kcpClient{
		cfg:         cfg,
		syncRoot:    syncRoot,
		syncRemote:  syncRemote,
		sessionsMux: new(sync.RWMutex),
	}

	s.smuxConfig = smux.DefaultConfig()
	s.smuxConfig.MaxReceiveBuffer = s.cfg.SmuxBuf
	s.smuxConfig.KeepAliveInterval = time.Duration(s.cfg.KeepAlive) * time.Second

	log.Infoln("initiating key derivation")
	pass := pbkdf2.Key([]byte(s.cfg.Key), []byte(SALT), 4096, 32, sha1.New)
	var block kcp.BlockCrypt
	switch s.cfg.Algo {
	case "sm4":
		block, _ = kcp.NewSM4BlockCrypt(pass[:16])
	case "tea":
		block, _ = kcp.NewTEABlockCrypt(pass[:16])
	case "xor":
		block, _ = kcp.NewSimpleXORBlockCrypt(pass)
	case "none":
		block, _ = kcp.NewNoneBlockCrypt(pass)
	case "aes-128":
		block, _ = kcp.NewAESBlockCrypt(pass[:16])
	case "aes-192":
		block, _ = kcp.NewAESBlockCrypt(pass[:24])
	case "blowfish":
		block, _ = kcp.NewBlowfishBlockCrypt(pass)
	case "twofish":
		block, _ = kcp.NewTwofishBlockCrypt(pass)
	case "cast5":
		block, _ = kcp.NewCast5BlockCrypt(pass[:16])
	case "3des":
		block, _ = kcp.NewTripleDESBlockCrypt(pass[:24])
	case "xtea":
		block, _ = kcp.NewXTEABlockCrypt(pass[:16])
	case "salsa20":
		block, _ = kcp.NewSalsa20BlockCrypt(pass)
	default:
		s.cfg.Algo = "aes"
		block, _ = kcp.NewAESBlockCrypt(pass)
	}
	s.blockCrypt = block

	return s
}

const (
	// SALT is use for pbkdf2 key expansion
	SALT = "beam-o"
)

func (s *kcpClient) StartBeam(ctx context.Context, totalCount int64, files <-chan string) {

	s.sessions = make([]sessionWithTTL, s.cfg.Streams)
	for k := range s.sessions {
		s.sessions[k].session = s.waitConn(ctx)
		s.sessions[k].ttl = time.Now().Add(time.Duration(s.cfg.AutoExpire) * time.Second)
	}

	s.chScavenger = make(chan *smux.Session, 128)
	go scavenger(s.chScavenger, s.cfg.ScavengeTTL)

	wg := new(sync.WaitGroup)
	wg.Add(s.cfg.Streams)
	retry := make(chan string, totalCount)

	s.beamPace = pace.New("teleported", 30*time.Second, TransferPaceReporter())
	defer func() {
		s.beamPace.Report(TransferPaceReporter())
		s.beamPace.Pause()
	}()
	for beamIdx := 0; beamIdx < s.cfg.Streams; beamIdx++ {
		go s.runBeam(ctx, wg, beamIdx, files, retry)
	}
	wg.Wait()
	close(retry)
	s.retryFiles(0, retry)
}

func (s *kcpClient) Config() *Config {
	return s.cfg
}

type sessionWithTTL struct {
	session *smux.Session
	ttl     time.Time
}

func (s *kcpClient) incProcessed() {
	atomic.AddInt64(&(s.processed), 1)
}

func (s *kcpClient) GetProcessed() int64 {
	return atomic.LoadInt64(&(s.processed))
}

func (s *kcpClient) runBeam(ctx context.Context, wg *sync.WaitGroup,
	beamIdx int, files <-chan string, retry chan string) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			log.Fatalln("interrupted")
		case file, ok := <-files:
			if !ok {
				return
			}
			w := s.obtainStream(beamIdx)
			if w == nil {
				log.WithFields(log.Fields{
					"beam_id": beamIdx,
				}).Warnln("failed to obtain stream: session is broken")
				retry <- file
				return
			}
			if err := s.writeFile(w, file); err != nil {
				retry <- file
			} else {
				s.incProcessed()
			}
			w.Close()
		}
	}
}

func (s *kcpClient) refreshSession(beamIdx int) {
	ctx, cancelFn := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFn()

	s.sessionsMux.RLock()
	session := s.sessions[beamIdx].session
	ttl := s.sessions[beamIdx].ttl
	s.sessionsMux.RUnlock()

	if session.IsClosed() || (s.cfg.AutoExpire > 0 && time.Now().After(ttl)) {
		s.chScavenger <- session

		session = s.waitConn(ctx)
		ttl = time.Now().Add(time.Duration(s.cfg.AutoExpire) * time.Second)

		s.sessionsMux.Lock()
		s.sessions[beamIdx].session = session
		s.sessions[beamIdx].ttl = ttl
		s.sessionsMux.Unlock()
	}
}

func (s *kcpClient) writeFile(w io.Writer, name string) error {
	w = NewWriterPace(s.beamPace, w)

	fullPath := filepath.Join(s.syncRoot, name)
	fnLog := log.WithFields(log.Fields{
		"path": fullPath,
	})

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		fnLog.WithError(err).Warnln("failed to stat file")
		return nil
	}
	fileMode := info.Mode()
	if fileMode&os.ModeSymlink == 1 {
		return nil
	}
	isDir := info.IsDir()

	seg := capn.NewBuffer(nil)
	hdr := proto.AutoNewFileHeader(seg)
	hdr.SetName(name)
	hdr.SetIsDir(isDir)
	hdr.SetMode(uint32(fileMode))
	hdr.SetModTime(info.ModTime().UnixNano())
	if !isDir {
		hdr.SetSize(info.Size())
	}
	if _, err := seg.WriteTo(w); err != nil {
		fnLog.WithError(err).Warnln("failed to beam file header")
		return errors.WithStack(err)
	}
	if isDir {
		return nil
	}
	f, err := os.Open(fullPath)
	if err != nil {
		fnLog.WithError(err).Warnln("failed to open local file")
		return errors.WithStack(err)
	}
	defer f.Close()

	buf := make([]byte, 65535)
	if _, err = io.CopyBuffer(w, f, buf); err != nil {
		fnLog.WithError(err).Warnln("failed to write file contents")
		return errors.WithStack(err)
	}
	return nil
}

func (s *kcpClient) retryFiles(beamIdx int, retry <-chan string) {
	for file := range retry {
		w := s.obtainStream(beamIdx)
		if w == nil {
			log.WithFields(log.Fields{
				"beam_id": beamIdx,
			}).Warnln("failed to obtain stream: session is broken")
			return
		}
		if err := s.writeFile(w, file); err == nil {
			s.incProcessed()
		}
	}
}

func (s *kcpClient) obtainStream(beamIdx int) *smux.Stream {
	for {
		s.sessionsMux.RLock()
		session := s.sessions[beamIdx].session
		s.sessionsMux.RUnlock()

		stream, err := session.OpenStream()
		if err != nil {
			log.WithFields(log.Fields{
				"beam_id": beamIdx,
			}).WithError(err).Warningln("failed to open stream in a beam session")
			session.Close()
			time.Sleep(time.Second)
			s.refreshSession(beamIdx)
			continue
		}
		return stream
	}
	return nil
}

func (s *kcpClient) createConn() (*smux.Session, error) {
	kcpconn, err := kcp.DialWithOptions(s.syncRemote, s.blockCrypt, s.cfg.DataShard, s.cfg.ParityShard)
	if err != nil {
		err = errors.Wrap(err, "kcp.DialWithOptions failed")
		return nil, err
	}
	kcpconn.SetStreamMode(true)
	kcpconn.SetWriteDelay(false)
	kcpconn.SetNoDelay(s.cfg.NoDelay, s.cfg.Interval, s.cfg.Resend, s.cfg.NoCongestion)
	kcpconn.SetWindowSize(s.cfg.SndWnd, s.cfg.RcvWnd)
	kcpconn.SetMtu(s.cfg.MTU)
	kcpconn.SetACKNoDelay(s.cfg.AckNodelay)

	if err := kcpconn.SetDSCP(s.cfg.DSCP); err != nil {
		log.WithError(err).Warnln("SetDSCP failed")
	}
	if err := kcpconn.SetReadBuffer(s.cfg.SockBuf); err != nil {
		log.WithError(err).Warnln("SetReadBuffer failed")
	}
	if err := kcpconn.SetWriteBuffer(s.cfg.SockBuf); err != nil {
		log.WithError(err).Warnln("SetWriteBuffer failed")
	}

	var session *smux.Session
	if s.cfg.NoComp {
		session, err = smux.Client(kcpconn, s.smuxConfig)
	} else {
		session, err = smux.Client(newCompStream(kcpconn), s.smuxConfig)
	}
	if err != nil {
		err = errors.Wrap(err, "smux.Client init failed")
		return nil, err
	}
	log.WithFields(log.Fields{
		"from": kcpconn.LocalAddr(),
		"to":   kcpconn.RemoteAddr(),
	}).Infoln("remote connection established")
	return session, nil
}

func (s *kcpClient) waitConn(ctx context.Context) *smux.Session {
	for {
		select {
		case <-ctx.Done():
			log.Fatalln("interrupted")
		default:
			session, err := s.createConn()
			if err != nil {
				log.Infoln("re-connecting:", err)
				time.Sleep(time.Second)
				continue
			}
			return session
		}
	}
}

type compStream struct {
	conn net.Conn
	w    *snappy.Writer
	r    *snappy.Reader
}

func (c *compStream) Read(p []byte) (n int, err error) {
	return c.r.Read(p)
}

func (c *compStream) Write(p []byte) (n int, err error) {
	n, err = c.w.Write(p)
	err = c.w.Flush()
	return n, err
}

func (c *compStream) Close() error {
	return c.conn.Close()
}

func newCompStream(conn net.Conn) *compStream {
	c := new(compStream)
	c.conn = conn
	c.w = snappy.NewBufferedWriter(conn)
	c.r = snappy.NewReader(conn)
	return c
}

type scavengeSession struct {
	session *smux.Session
	ts      time.Time
}

func scavenger(ch chan *smux.Session, ttl int) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var sessionList []scavengeSession
	for {
		select {
		case sess := <-ch:
			sessionList = append(sessionList, scavengeSession{sess, time.Now()})
			log.Debugln("session marked as expired")
		case <-ticker.C:
			var newList []scavengeSession
			for k := range sessionList {
				s := sessionList[k]
				if s.session.NumStreams() == 0 || s.session.IsClosed() {
					log.Debugln("session normally closed")
					s.session.Close()
				} else if ttl >= 0 && time.Since(s.ts) >= time.Duration(ttl)*time.Second {
					log.Debugln("session reached scavenge ttl")
					s.session.Close()
				} else {
					newList = append(newList, sessionList[k])
				}
			}
			sessionList = newList
		}
	}
}
