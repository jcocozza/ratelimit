package ratelimit

type RequestOption func(*requestOptions)

type requestOptions struct {
	// a throttleLimit of 0 means make all requests
	// don't make the request if you want to throttle to 0
	throttleLimit *int
}

// throttle of 0 will allow no requests
//
// throttle of <= 0 will still be 0
func WithThrottle(limit int) RequestOption {
	if limit <= 0 {
		limit = 0
	}
	return func(o *requestOptions) {
		o.throttleLimit = &limit
	}
}

func createOptions(opts []RequestOption) *requestOptions {
	options := &requestOptions{}
	for _, opt := range opts {
		opt(options)
	}
	return options
}
