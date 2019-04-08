package server

import (
	"io"
	"io/ioutil"
	"net"
	"time"

	"github.com/golang/snappy"
	log "github.com/sirupsen/logrus"
	kcp "github.com/xtaci/kcp-go"
	"github.com/xtaci/smux"
)

type Server interface {
	Start()
}

type kcpServer struct {
	cfg      *Config
	l        *kcp.Listener
	syncRoot string
}

func NewKCP(cfg *Config, l *kcp.Listener, syncRoot string) Server {
	return &kcpServer{
		cfg:      cfg,
		l:        l,
		syncRoot: syncRoot,
	}
}

func (s *kcpServer) Start() {
	for {
		if conn, err := s.l.AcceptKCP(); err == nil {
			log.WithFields(log.Fields{
				"remote_addr": conn.RemoteAddr(),
			}).Infoln("accepting remote conn")

			conn.SetStreamMode(true)
			conn.SetWriteDelay(false)
			conn.SetNoDelay(s.cfg.NoDelay, s.cfg.Interval, s.cfg.Resend, s.cfg.NoCongestion)
			conn.SetMtu(s.cfg.MTU)
			conn.SetWindowSize(s.cfg.SndWnd, s.cfg.RcvWnd)
			conn.SetACKNoDelay(s.cfg.AckNodelay)

			if s.cfg.NoComp {
				go s.handleSession(conn)
			} else {
				go s.handleSession(newCompStream(conn))
			}
		} else {
			log.WithError(err).Warnln("failed to accept KCP conn")
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

func (s *kcpServer) handleSession(conn io.ReadWriteCloser) {
	smuxConfig := smux.DefaultConfig()
	smuxConfig.MaxReceiveBuffer = s.cfg.SmuxBuf
	smuxConfig.KeepAliveInterval = time.Duration(s.cfg.KeepAlive) * time.Second

	mux, err := smux.Server(conn, smuxConfig)
	if err != nil {
		log.WithError(err).Warnln("failed to start smux server")
		return
	}
	defer mux.Close()

	for {
		stream, err := mux.AcceptStream()
		if err != nil {
			log.WithError(err).Warnln("failed to accept stream")
			return
		}
		io.Copy(ioutil.Discard, stream)
		stream.Close()
		// _ = stream // deal with stream
		// read header, determine, process
		// if err != nil {
		// 	stream.Close()
		// log.Warnln()
		// continue
		// }
		// go todo(stream)
	}
}

// func handleClient(p1, p2 io.ReadWriteCloser) {
// 	defer p1.Close()
// 	defer p2.Close()

// 	// start tunnel & wait for tunnel termination
// 	streamCopy := func(dst io.Writer, src io.Reader) chan struct{} {
// 		die := make(chan struct{})
// 		go func() {
// 			io.CopyBuffer(dst, src, make([]byte, 65535))
// 			close(die)
// 		}()
// 		return die
// 	}

// 	select {
// 	case <-streamCopy(p1, p2):
// 	case <-streamCopy(p2, p1):
// 	}
// }
