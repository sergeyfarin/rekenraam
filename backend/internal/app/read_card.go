package app

// ReadCard holds one summary or the error that prevented it from being read.
// A composed read can return the other cards even when this one fails.
type ReadCard[T any] struct {
	Summary *T
	Err     error
}

func readCard[T any](summary *T, err error) ReadCard[T] {
	if err != nil {
		return ReadCard[T]{Err: err}
	}
	return ReadCard[T]{Summary: summary}
}

func valueReadCard[T any](summary T, err error) ReadCard[T] {
	return readCard(&summary, err)
}
