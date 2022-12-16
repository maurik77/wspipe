package connection

import "time"

type Option struct {
	applyOption func(*connectionInstance) *connectionInstance
}

func WithWriteWaitTimeout(d time.Duration) Option {
	return Option{
		applyOption: func(c *connectionInstance) *connectionInstance {
			c.writeWait = d
			return c
		},
	}
}

func WithPongWaitTimeout(d time.Duration) Option {
	return Option{
		applyOption: func(c *connectionInstance) *connectionInstance {
			c.pongWait = d
			return c
		},
	}
}

func WithPingPeriod(d time.Duration) Option {
	return Option{
		applyOption: func(c *connectionInstance) *connectionInstance {
			c.pingPeriod = d
			return c
		},
	}
}

func WithMaxMessageSize(size int64) Option {
	return Option{
		applyOption: func(c *connectionInstance) *connectionInstance {
			c.maxMessageSize = size
			return c
		},
	}
}
