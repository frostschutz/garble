package main

import (
	cryptorand "crypto/rand"
	"crypto/sha512"
	"flag"
	"fmt"
	//	"math/rand"
	"os"
	"runtime"
	"time"
)

const (
	BSIZE = 65536
)

var (
	narg   = int(0)
	phrase = ""
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
