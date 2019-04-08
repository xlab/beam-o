package client

import (
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"

	toml "github.com/pelletier/go-toml"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type Config struct {
	Pprof   bool `toml:"beam.pprof" default:"false"`
	Streams int  `toml:"beam.streams" default:"0"`

	NoComp      bool `toml:"session.nocomp" default:"false"`
	SmuxBuf     int  `toml:"session.smuxbuf" default:"4194304"`
	KeepAlive   int  `toml:"session.keepalive" default:"10"`
	ScavengeTTL int  `toml:"session.scavengettl" default:"600"`
	AutoExpire  int  `toml:"session.autoexpire" default:"0"`

	AckNodelay   bool   `toml:"kcp.acknodelay" default:"false"`
	NoDelay      int    `toml:"kcp.nodelay" default:"0"`
	Resend       int    `toml:"kcp.resend" default:"0"`
	NoCongestion int    `toml:"kcp.nc" default:"0"`
	Interval     int    `toml:"kcp.interval" default:"50"`
	SndWnd       int    `toml:"kcp.sndwnd" default:"1024"`
	RcvWnd       int    `toml:"kcp.rcvwnd" default:"1024"`
	Mode         string `toml:"kcp.mode" default:"fast"`

	DataShard   int    `toml:"crypt.datashard" default:"10"`
	ParityShard int    `toml:"crypt.parityshard" default:"3"`
	Key         string `toml:"crypt.key" default:"it's a secrect"`
	Algo        string `toml:"crypt.algo" default:"aes"`

	DSCP    int `toml:"udp.dscp" default:"0"`
	MTU     int `toml:"udp.mtu" default:"1350"`
	SockBuf int `toml:"udp.sockbuf" default:"4194304"`
}

func DefaultConfig() *Config {
	cfg := Config{}
	toml.Unmarshal([]byte{}, &cfg)
	cfg = cfg.overrideFromEnv()
	cfg.checkConfig()
	return &cfg
}

func ReadConfig(path string) (*Config, error) {
	fnLog := log.WithFields(log.Fields{
		"fn": "ReadConfig",
	})

	configTree, err := toml.LoadFile(path)
	if err != nil {
		err = errors.Wrap(err, "failed to read config file")
		return nil, err
	}
	cfg := Config{}
	if err := configTree.Unmarshal(&cfg); err != nil {
		fnLog.WithError(err).Warnln("failed to unmarshal config, using defaults")
		toml.Unmarshal([]byte{}, &cfg)
	}

	cfg = cfg.overrideFromEnv()
	cfg.checkConfig()
	return &cfg, nil
}

func (cfg *Config) overrideFromEnv() Config {
	fnLog := log.WithFields(log.Fields{
		"fn": "overrideFromEnv",
	})

	data, _ := toml.Marshal(cfg)
	configTree, _ := toml.LoadBytes(data)

	allKeys := keysFlat("", configTree)
	for _, key := range allKeys {
		envKey := configKeyToEnv(key)
		envValue, ok := os.LookupEnv(envKey)
		if !ok {
			continue
		}
		configValue, err := configTypecastValue(configTree.Get(key), envValue)
		if err != nil {
			fnLog.WithError(err).Warnf("incompatible value of %s with config key", envKey)
			continue
		}
		fnLog.Debugf("overriding config key with %s env value", envKey)
		configTree.Set(key, configValue)
	}

	newConfig := Config{}
	configTree.Unmarshal(&newConfig)
	return newConfig
}

func (cfg *Config) checkConfig() *Config {
	if cfg.Streams == 0 {
		cfg.Streams = runtime.NumCPU()
	}
	switch cfg.Mode {
	case "normal":
		cfg.NoDelay, cfg.Interval, cfg.Resend, cfg.NoCongestion = 0, 40, 2, 1
	case "fast":
		cfg.NoDelay, cfg.Interval, cfg.Resend, cfg.NoCongestion = 0, 30, 2, 1
	case "fast2":
		cfg.NoDelay, cfg.Interval, cfg.Resend, cfg.NoCongestion = 1, 20, 2, 1
	case "fast3":
		cfg.NoDelay, cfg.Interval, cfg.Resend, cfg.NoCongestion = 1, 10, 2, 1
	default:
		cfg.Mode = "fast"
		return cfg.checkConfig()
	}
	return cfg
}

func keysFlat(prefix string, tree *toml.Tree) []string {
	var traverseTree func(prefix string, t *toml.Tree) []string
	traverseTree = func(prefix string, t *toml.Tree) []string {
		var finalKeys []string
		keys := t.Keys()
		for _, k := range keys {
			if tt, ok := t.Get(k).(*toml.Tree); ok {
				subkeys := traverseTree(k, tt)
				finalKeys = append(finalKeys, subkeys...)
				continue
			}
			if len(prefix) > 0 {
				finalKeys = append(finalKeys, prefix+"."+k)
				continue
			}
			finalKeys = append(finalKeys, k)
		}
		return finalKeys
	}
	keys := traverseTree("", tree)
	sort.Strings(keys)
	return keys
}

func configTypecastValue(to interface{}, fromStr string) (interface{}, error) {
	switch to.(type) {
	case string:
		return fromStr, nil
	case int64:
		v, err := strconv.ParseInt(fromStr, 10, 64)
		if err != nil {
			err = errors.Wrapf(err, "failed to parse int64 from %s", fromStr)
		}
		return v, err
	case uint64:
		v, err := strconv.ParseUint(fromStr, 10, 64)
		if err != nil {
			err = errors.Wrapf(err, "failed to parse uint64 from %s", fromStr)
		}
		return v, err
	case float64:
		v, err := strconv.ParseFloat(fromStr, 64)
		if err != nil {
			err = errors.Wrapf(err, "failed to parse float64 from %s", fromStr)
		}
		return v, err
	case bool:
		switch fromStr {
		case "1", "true", "True", "TRUE", "YES", "Yes", "yes", "Y", "y":
			return true, nil
		case "", "0", "false", "False", "FALSE", "NO", "No", "no", "N", "n":
			return false, nil
		}
		err := errors.Errorf("failed to parse bool from %s", fromStr)
		return false, err
	// case time.Time:
	// case []string:
	// case []int64:
	// case []uint64:
	// case []float64:
	// case []bool:
	// case []time.Time:
	default:
		err := errors.Errorf("unsupported type %T", to)
		return nil, err
	}
}

func configKeyToEnv(key string) string {
	key = strings.Replace(key, ".", "_", -1)
	key = strings.ToUpper(key)
	return "BEAM_CLIENT_" + key
}
