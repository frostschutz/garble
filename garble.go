package main

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"runtime"
	"time"
)

const (
	BSIZE    = 65536
	BSIZE7   = BSIZE - BSIZEMOD
	BSIZEMOD = BSIZE % 7
	MULTI    = 4
	SOURCES  = 8
	POOL     = SOURCES * (MULTI + 1)
)

var (
	narg   = int(0)
	phrase = ""
	pool   = make(chan []byte, POOL)
	loop   = make(chan []byte, POOL)
	data   = make([]chan []byte, SOURCES)
)

// randomSeed produces a int64 seed based on crypto/rand and time.
func randomSeed() int64 {
	var seed int64

	urandom := make([]byte, 8)
	cryptorand.Reader.Read(urandom)

	for key, value := range urandom {
		seed ^= (int64(value) ^ time.Now().UTC().UnixNano()) << (uint(key) * 8)
	}

	return seed
}

// randomBytes fills byte buffers with random data
func randomBytes(src rand.Source, out chan<- []byte) {
	var (
		r int64
		i = BSIZE
	)

	for buf, ok := <-pool; ok; buf, ok = <-pool {
		r = src.Int63()
		switch { // Go seems to eliminate impossible cases
		case BSIZEMOD == 6:
			buf[BSIZE-6] = byte(r >> 48)
			fallthrough
		case BSIZEMOD == 5:
			buf[BSIZE-5] = byte(r >> 32)
			fallthrough
		case BSIZEMOD == 4:
			buf[BSIZE-4] = byte(r >> 24)
			fallthrough
		case BSIZEMOD == 3:
			buf[BSIZE-3] = byte(r >> 16)
			fallthrough
		case BSIZEMOD == 2:
			buf[BSIZE-2] = byte(r >> 8)
			fallthrough
		case BSIZEMOD == 1:
			buf[BSIZE-1] = byte(r)
		}

		for i = 0; i < BSIZE7; i += 7 {
			r = src.Int63()
			buf[i] = byte(r)
			buf[i+1] = byte(r >> 8)
			buf[i+2] = byte(r >> 16)
			buf[i+3] = byte(r >> 24)
			buf[i+4] = byte(r >> 32)
			buf[i+5] = byte(r >> 40)
			buf[i+6] = byte(r >> 48)
		}

		out <- buf
	}
}

func garble(f *os.File, in <-chan []byte, out chan<- bool) {
	var (
		data []byte // data we read
		err  error  // I/O error
		h    int    // data index
		n    int    // data size
		pos  int64  // file pos
	)

	data = make([]byte, BSIZE)
	err = nil
	h = 0
	n = 0
	pos = 0

	for buf, ok := <-in; ok; buf, ok = <-in {
		for _, randombyte := range buf {
			if h == n {
				if h > 0 {
					f.WriteAt(data[0:h], pos)
					pos += int64(h)
				}

				h = 0
				n, err = f.Read(data)

				if err != nil && err != io.EOF {
					panic(err)
				}
				if n == 0 {
					close(out)
					for {
						<-in // sleep of no return
					}
				}
			}

			data[h] ^= randombyte
			h++
		}

		out <- true
		// close(out)
	}
}

func init() {
	flag.StringVar(&phrase, "phrase", "", "the Garble phrase, by default random")
	flag.Parse()

	narg = flag.NArg()

	if narg <= 0 {
		flag.Usage()
		os.Exit(1)
	}

	if phrase == "" {
		phrase = fmt.Sprintf("%016x", uint64(randomSeed()))
	}

	fmt.Println("Using phrase:", phrase)
}

func main() {
	// Use available CPUs:
	if runtime.GOMAXPROCS(0) == 1 &&
		runtime.NumCPU() > 1 &&
		os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	// Open files:
	files := make([]*os.File, narg)
	writers := make([]chan []byte, narg)
	signals := make([]chan bool, narg)

	for i, arg := range flag.Args() {
		f, err := os.OpenFile(arg, os.O_RDWR, 0666)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		files[i] = f
		writers[i] = make(chan []byte, MULTI)
		signals[i] = make(chan bool, MULTI)
		go garble(files[i], writers[i], signals[i])
	}

	// Allocate byte buffer pool:
	buffer := make([]byte, BSIZE*POOL)

	for i := 0; i < BSIZE*POOL; i += BSIZE {
		pool <- buffer[i : i+BSIZE]
	}

	// Initialize random sources:
	hash := sha512.New()
	sum := make([]byte, hash.Size())

	for i := 0; i < SOURCES; i++ {
		var seed, s int64
		var err error

		hash.Write([]byte(":garble:" + phrase))
		hash.Sum(sum[:0])

		buf := bytes.NewReader(sum)
		s = 0

		for err == nil {
			err = binary.Read(buf, binary.LittleEndian, &s)
			seed ^= s
		}

		src := rand.NewSource(seed)
		data[i] = make(chan []byte, MULTI)
		go randomBytes(src, data[i])
	}

	// Route data channels:
	go func(data []chan []byte, writers []chan []byte, signals []chan bool) {
		var buf []byte
		for {
			for _, r := range data {
				buf = <-r

				for i, w := range writers {
					if signals[i] != nil {
						w <- buf
					}
				}

				loop <- buf
			}
		}
	}(data, writers, signals)

	var buf []byte

	for narg > 0 {
		buf = <-loop

		for i, s := range signals {
			if signals[i] != nil {
				_, ok := <-s

				if !ok {
					signals[i] = nil
					narg--
				}
			}
		}

		pool <- buf
	}

	fmt.Println("All done!")
}
