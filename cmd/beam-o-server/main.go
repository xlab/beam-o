package main

import (
	"crypto/sha1"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"time"

	cli "github.com/jawher/mow.cli"
	log "github.com/sirupsen/logrus"
	"github.com/xlab/beam-o/server"
	kcp "github.com/xtaci/kcp-go"
	"golang.org/x/crypto/pbkdf2"
)

var (
	// VERSION is injected by buildflags
	VERSION = "SELFBUILD"
	// SALT is use for pbkdf2 key expansion
	SALT = "beam-o"
)

var app = cli.App("beam-o-server", "Accepts beams that are carrying bytes from some distant galaxy.")

var (
	syncRoot = app.StringArg("SYNCROOT", "", "Specify the sync root")
	cfgPath  = app.StringOpt("cfg", "", "Specify config path (default SYNCROOT/.beam/config_server.toml)")
)

func main() {
	rand.Seed(int64(time.Now().Nanosecond()))
	if VERSION == "SELFBUILD" {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}
	app.Version("v version", VERSION)
	app.Action = func() {
		var cfg *server.Config
		if len(*cfgPath) == 0 {
			*cfgPath = filepath.Join(*syncRoot, ".beam", "config.toml")
			if _, err := os.Stat(*cfgPath); err == nil {
				if c, err := server.ReadConfig(*cfgPath); err != nil {
					log.WithError(err).Warnln("failed to load config, using default")
					cfg = server.DefaultConfig()
				} else {
					cfg = c
				}
			} else {
				cfg = server.DefaultConfig()
			}
		} else if c, err := server.ReadConfig(*cfgPath); err != nil {
			log.WithError(err).Warnln("failed to load config, using default")
			cfg = server.DefaultConfig()
		} else {
			cfg = c
		}
		log.Infoln("version", VERSION)

		log.Infoln("initiating key derivation")
		pass := pbkdf2.Key([]byte(cfg.Key), []byte(SALT), 4096, 32, sha1.New)
		var block kcp.BlockCrypt
		switch cfg.Algo {
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
			cfg.Algo = "aes"
			block, _ = kcp.NewAESBlockCrypt(pass)
		}

		l, err := kcp.ListenWithOptions(cfg.Listen, block, cfg.DataShard, cfg.ParityShard)
		if err != nil {
			log.WithFields(log.Fields{
				"laddr": cfg.Listen,
			}).WithError(err).Fatalln("failed to start listener")
		} else {
			log.WithFields(log.Fields{
				"laddr": l.Addr().String(),
				"cfg":   cfg.Listen,
			}).Infoln("started KCP listener")
		}
		if err := l.SetDSCP(cfg.DSCP); err != nil {
			log.WithError(err).Warnln("SetDSCP failed")
		}
		if err := l.SetReadBuffer(cfg.SockBuf); err != nil {
			log.WithError(err).Warnln("SetReadBuffer failed")
		}
		if err := l.SetWriteBuffer(cfg.SockBuf); err != nil {
			log.WithError(err).Warnln("SetWriteBuffer failed")
		}
		if cfg.Pprof {
			go http.ListenAndServe(":6060", nil)
		}
		s := server.NewKCP(cfg, l, *syncRoot)
		s.Start()
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatalln(err)
	}
}
