package stopper

import (
	"sync"
	"testing"
)

func TestStopper(t *testing.T) {

	stopper := New()

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			<-stopper.ShouldStop()
		}()
	}

	stopper.Stop()

	wg.Wait()
}
