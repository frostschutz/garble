package main

import (
	cryptorand "crypto/rand"
	"crypto/sha512"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"time"
)

const (
	BSIZE    = 65536
	BSIZEMOD = BSIZE % 7
	BSIZE7   = BSIZE - BSIZEMOD
	SRC      = 8
)

var (
	narg    = int(0)
	phrase  = ""
	sources [8]rand.Source
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

func randomBytes(src rand.Source) []byte {
	buf := make([]byte, BSIZE)

	for i := 0; i < BSIZE7; i += 7 {
		r := src.Int63()
		buf[i] = byte(r)
		buf[i+1] = byte(r << 8)
		buf[i+2] = byte(r << 16)
		buf[i+3] = byte(r << 24)
		buf[i+4] = byte(r << 32)
		buf[i+5] = byte(r << 40)
		buf[i+6] = byte(r << 48)
	}

	r := src.Int63()

	for i := BSIZE - BSIZE7; i < BSIZE; i++ {
		buf[i] = byte(r)
		r = r << 8
	}

	return buf
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
}

func garble(index int, f *os.File, c chan []byte) {
	fmt.Println("garble(", index, f, c, ")")
	<-c
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	channels := make([]chan []byte, narg)
	files := make([]*os.File, narg)

	for i, arg := range flag.Args() {
		f, err := os.OpenFile(arg, os.O_RDWR, 0666)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		files[i] = f
		channels[i] = make(chan []byte, 8)
		go garble(i, files[i], channels[i])
	}

	hash := sha512.New()
	seed := []byte(phrase)
	b := make([]byte, hash.Size())
	for i := 0; i < 2; i++ {
		hash.Write(seed)
		hash.Sum(b[:0])
		fmt.Println(b)
	}
}
