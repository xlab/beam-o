package main

import (
	"context"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	cli "github.com/jawher/mow.cli"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/buntdb"
	"github.com/xlab/closer"
	"github.com/xlab/pace"

	"github.com/xlab/beam-o/client"
)

var (
	// VERSION is injected by buildflags
	VERSION = "SELFBUILD"
)

var app = cli.App("beam-o-client", "Fires a beam to teleport your files onto servers in distant galaxies.")

var (
	syncRoot   = app.StringArg("SYNCROOT", "", "Specify the sync root")
	syncRemote = app.StringArg("SYNCREMOTE", "remote:29999", "Specify the remote host UDP address")
	statePath  = app.StringOpt("index", "", "State database path (default SYNCROOT/.beam/index.db)")
	cfgPath    = app.StringOpt("cfg", "", "Specify config path (default SYNCROOT/.beam/config_client.toml)")
	saveState  = app.BoolOpt("w", false, "Save state after building index")
	readState  = app.BoolOpt("r", false, "Read state instead of building index")
)

func main() {
	rand.Seed(int64(time.Now().Nanosecond()))
	if VERSION == "SELFBUILD" {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
	app.Spec = "[OPTIONS] SYNCROOT SYNCREMOTE"

	app.Version("v version", VERSION)
	app.Action = func() {
		defer closer.Close()

		log.Infoln("version", VERSION)

		kcpClient := initClient()
		if kcpClient.Config().Pprof {
			go http.ListenAndServe(":6060", nil)
		}

		if !filepath.IsAbs(*syncRoot) {
			*syncRoot, _ = filepath.Abs(*syncRoot)
		}
		if len(*statePath) == 0 {
			*statePath = filepath.Join(*syncRoot, ".beam", "index.db")
		}

		var db *buntdb.DB
		var buildIndex bool
		if *readState {
			db = readIndex(*statePath)
			if db == nil {
				buildIndex = true
			}
		}
		if buildIndex {
			memIdx, err := buntdb.Open(":memory:")
			if err != nil {
				log.WithError(err).Fatalln("failed to init index")
			} else {
				db = memIdx
			}
		}
		closer.Bind(func() {
			if err := db.Close(); err != nil {
				log.WithError(err).Warnln("failed to close index")
			}
		})

		if buildIndex {
			files := make(chan string, runtime.NumCPU()*10000)

			readStop := make(chan bool, 1)
			writeDone := make(chan bool, 1)
			closer.Bind(func() {
				close(readStop)
				<-writeDone
			})

			go runIndexBuild(db, files, readStop, writeDone)

			var warnings int
			ts := time.Now()
			beamRoot := filepath.Join(*syncRoot, ".beam")
			if err := filepath.Walk(*syncRoot, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					warnings++
					log.Warningln("skipping", path)
					return nil
				}
				if path == beamRoot {
					log.Infoln("skipping", path)
					return nil
				}
				path = strings.TrimPrefix(path, *syncRoot)
				files <- path
				return nil
			}); err != nil {
				log.WithError(err).Fatalln("failed to scan sync root")
			}
			close(files)
			<-writeDone
			log.Println("index built in", time.Since(ts))
			log.Println("total warnings", warnings)
		}
		if buildIndex && *saveState {
			runSaveIndex(db, *statePath)
		}

		ctx, cancelFn := context.WithCancel(context.Background())
		closer.Bind(func() {
			cancelFn()
		})

		var totalCount int64
		if err := db.View(func(tx *buntdb.Tx) error {
			countStr, _ := tx.Get("_count")
			totalCount, _ = strconv.ParseInt(countStr, 10, 64)
			if totalCount == 0 {
				err := errors.New("index seems empty")
				return err
			}

			files := make(chan string, runtime.NumCPU()*1024)
			go func() {
				defer close(files)

				if err := tx.Ascend("", func(_, v string) bool {
					select {
					case <-ctx.Done():
						return false
					case files <- v:
						return true
					}
				}); err != nil {
					log.WithError(err).Warnln("failed to scan index entries")
				}
			}()

			kcpClient.StartBeam(ctx, totalCount, files)
			return nil
		}); err != nil {
			log.WithError(err).Warnln("db scan failed")
		}

		if processed := kcpClient.GetProcessed(); processed != totalCount {
			log.WithFields(log.Fields{
				"total":     totalCount,
				"processed": processed,
				"failed":    totalCount - processed,
			}).Fatalln("all streams are done, but some files are not synced")
		}
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatalln(err)
	}
}

func initClient() client.Client {
	var cfg *client.Config
	if len(*cfgPath) == 0 {
		*cfgPath = filepath.Join(*syncRoot, ".beam", "config.toml")
		if _, err := os.Stat(*cfgPath); err == nil {
			if c, err := client.ReadConfig(*cfgPath); err != nil {
				log.WithError(err).Warnln("failed to load config, using default")
				cfg = client.DefaultConfig()
			} else {
				cfg = c
			}
		} else {
			cfg = client.DefaultConfig()
		}
	} else if c, err := client.ReadConfig(*cfgPath); err != nil {
		log.WithError(err).Warnln("failed to load config, using default")
		cfg = client.DefaultConfig()
	} else {
		cfg = c
	}

	return client.NewKCP(cfg, *syncRoot, *syncRemote)
}

func readIndex(path string) *buntdb.DB {
	db, err := buntdb.Open(path)
	if err != nil {
		log.Infoln("previous index could not be read, will be built")
		return nil
	}
	var totalCount int
	err = db.View(func(tx *buntdb.Tx) error {
		totalCountStr, _ := tx.Get("_count")
		totalCount, _ = strconv.Atoi(totalCountStr)
		return nil
	})
	if err != nil {
		log.WithError(err).Warningln("index seems to be broken, will rebuild")
		db.Close()
		return nil
	}
	if totalCount == 0 {
		log.WithFields(log.Fields{
			"total_count": totalCount,
		}).Warningln("index seems to be empty, will rebuild")
		db.Close()
		return nil
	}
	log.Infoln("loaded previous index with", totalCount, "entities")
	return db
}

func runSaveIndex(db *buntdb.DB, path string) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		log.WithError(err).Warnln("failed to save index on disk")
		return
	}

	ts := time.Now()
	f, err := os.Create(path)
	if err != nil {
		log.WithError(err).Warnln("failed to save index on disk")
		return
	}
	defer f.Close()

	if err := db.Save(f); err != nil {
		log.WithError(err).Warnln("failed to save index on disk")
		return
	} else {
		log.Infoln("written index to disk in", time.Since(ts))
	}
	if info, err := os.Stat(path); err == nil {
		if mibs := float64(info.Size()) / (1024 * 1024); mibs >= 0.01 {
			log.Infof("index size %.2f MiB", mibs)
		}
	}
}

func runIndexBuild(db *buntdb.DB, files chan string, readStop, writeDone chan bool) {
	p := pace.New("indexed", 30*time.Second, client.PaceReporter())
	defer func() {
		p.Report(client.PaceReporter())
		p.Pause()
	}()

	tx, err := db.Begin(true)
	if err != nil {
		log.WithError(err).Fatalln("failed to begin Tx")
	}
	flushTx := func() {
		if err := tx.Commit(); err != nil {
			log.WithError(err).Fatalln("failed to commit Tx")
		}
		tx, err = db.Begin(true)
		if err != nil {
			log.WithError(err).Fatalln("failed to begin Tx")
		}
	}

	var totalCount int
	defer func() {
		log.Println("total index entries", totalCount)
	}()
	t := time.NewTimer(2 * time.Second)
	for {
		select {
		case <-readStop:
			tx.Set("_count", strconv.Itoa(totalCount), nil)
			if totalCount > 0 {
				if err := tx.Commit(); err != nil {
					log.WithError(err).Fatalln("failed to commit Tx")
				}
			} else if err := tx.Rollback(); err != nil {
				log.Warnln(err)
			}
			close(writeDone)
			return
		case path, ok := <-files:
			if !ok {
				tx.Set("_count", strconv.Itoa(totalCount), nil)
				if totalCount > 0 {
					if err := tx.Commit(); err != nil {
						log.WithError(err).Fatalln("failed to commit Tx")
					}
				} else if err := tx.Rollback(); err != nil {
					log.Warnln(err)
				}
				close(writeDone)
				return
			}
			id := client.GetULID()
			_, replaced, err := tx.Set(id, path, nil)
			if replaced {
				log.Fatalln("broken index")
			}
			if err != nil {
				log.WithFields(log.Fields{
					"filepath": path,
				}).WithError(err).Fatalln("failed to write file info to index")
			}
			p.StepN(1)
			totalCount++
		case <-t.C:
			flushTx()
			t.Reset(2 * time.Second)
		}
	}
}
