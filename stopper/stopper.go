package stopper

type Stopper struct {
	c chan struct{}
}

func New() *Stopper {
	return &Stopper{
		c: make(chan struct{}),
	}
}

func (s *Stopper) Stop() {
	close(s.c)
}

func (s *Stopper) ShouldStop() <-chan struct{} {
	return s.c
}
