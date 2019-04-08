package client

import (
	"io"
	"math/rand"
	"strconv"
	"sync"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/oklog/ulid"
	log "github.com/sirupsen/logrus"
	"github.com/xlab/pace"
)

var globalRand = rand.New(&lockedSource{
	src: rand.NewSource(time.Now().UnixNano()),
})

// GetULID constucts an Universally Unique Lexicographically Sortable Identifier.
// See https://github.com/oklog/ulid
func GetULID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), globalRand).String()
}

type lockedSource struct {
	lk  sync.Mutex
	src rand.Source
}

func (r *lockedSource) Int63() (n int64) {
	r.lk.Lock()
	n = r.src.Int63()
	r.lk.Unlock()
	return
}

func (r *lockedSource) Seed(seed int64) {
	r.lk.Lock()
	r.src.Seed(seed)
	r.lk.Unlock()
}

// PaceReporter reports using logrus.Printf and stops reporting when flow of events is stoped.
func PaceReporter() pace.ReporterFunc {
	var previous float64
	var stalled time.Time
	return func(label string, timeframe time.Duration, value float64) {
		switch {
		case value == 0 && previous == 0:
			return // don't report anything
		case value == 0 && previous != 0:
			dur := timeframe
			if !stalled.IsZero() {
				dur = time.Since(stalled)
				n := dur / timeframe
				if dur-n*timeframe < 10*time.Millisecond {
					dur = n * timeframe
				}
			} else {
				stalled = time.Now().Add(-dur)
			}
			log.Infof("%s: stalled for %v", label, dur)
			return
		default:
			previous = value
			stalled = time.Time{}
		}
		floatFmt := func(f float64) string {
			return strconv.FormatFloat(f, 'f', 3, 64)
		}
		switch timeframe {
		case time.Second:
			log.Infof("%s: %s/s in %v", label, floatFmt(value), timeframe)
		case time.Minute:
			log.Infof("%s: %s/m in %v", label, floatFmt(value), timeframe)
		case time.Hour:
			log.Infof("%s: %s/h in %v", label, floatFmt(value), timeframe)
		case 24 * time.Hour:
			log.Infof("%s: %s/day in %v", label, floatFmt(value), timeframe)
		default:
			log.Infof("%s %s in %v (pace: %s/s)", floatFmt(value), label,
				timeframe, floatFmt(value/(float64(timeframe)/float64(time.Second))))
		}
	}
}

func TransferPaceReporter() pace.ReporterFunc {
	var previous float64
	var stalled time.Time
	return func(label string, timeframe time.Duration, value float64) {
		switch {
		case value == 0 && previous == 0:
			return // don't report anything
		case value == 0 && previous != 0:
			dur := timeframe
			if !stalled.IsZero() {
				dur = time.Since(stalled)
				n := dur / timeframe
				if dur-n*timeframe < 10*time.Millisecond {
					dur = n * timeframe
				}
			} else {
				stalled = time.Now().Add(-dur)
			}
			log.Infof("%s: stalled for %v", label, dur)
			return
		default:
			previous = value
			stalled = time.Time{}
		}
		valueFmt := func(f float64) string {
			return humanize.Bytes(uint64(f))
		}
		switch timeframe {
		case time.Second:
			log.Infof("%s: %s/s in %v", label, valueFmt(value), timeframe)
		case time.Minute:
			log.Infof("%s: %s/m in %v", label, valueFmt(value), timeframe)
		case time.Hour:
			log.Infof("%s: %s/h in %v", label, valueFmt(value), timeframe)
		case 24 * time.Hour:
			log.Infof("%s: %s/day in %v", label, valueFmt(value), timeframe)
		default:
			log.Infof("%s %s in %v (pace: %s/s)", valueFmt(value), label,
				timeframe, valueFmt(value/(float64(timeframe)/float64(time.Second))))
		}
	}
}

func NewWriterPace(p pace.Pace, w io.Writer) io.Writer {
	return &writerPace{
		Writer: w,

		p: p,
	}
}

type writerPace struct {
	io.Writer

	p pace.Pace
}

func (wp *writerPace) Write(buf []byte) (int, error) {
	n, err := wp.Writer.Write(buf)
	wp.p.StepN(n)
	return n, err
}
