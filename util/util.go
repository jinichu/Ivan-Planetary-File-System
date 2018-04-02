package util

import (
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/stopper"
	"testing"
	"time"
)

func SucceedsSoon(t *testing.T, f func() error) {
	timeout := time.After(time.Second * 10)
	c := make(chan error)
	stop := stopper.New()

	go func() {
		for {
			select {
			case <-stop.ShouldStop():
				return
			case <-timeout:
				return
			default:
			}
			err := f()
			c <- err
			if err == nil {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	var err error
	for {
		select {
		case err = <-c:
			if err == nil {
				return
			}
		case <-timeout:
			stop.Stop()
			t.Fatalf("%+v", err)
			return
		}
	}
}
