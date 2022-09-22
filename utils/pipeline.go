package utils

import "sync"

// splits the chan returned from f into separate work chans
// num is the number of chans to split into f is the func that returns the chan
func Split[V any](num uint8, f func() chan V) []chan V {
	out := make([]chan V, num)
	for i := uint8(0); i < num; i++ {
		out[i] = f()
	}
	return out
}

// merges separate chans into a single chan
func Merge[V any](chs []chan V) chan V {
	wg := sync.WaitGroup{}
	out := make(chan V)
	send := func(c chan V) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}

	wg.Add(len(chs))
	for _, c := range chs {
		go send(c)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
