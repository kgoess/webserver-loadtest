package ringbuffer

import "time"
import "log"

// debugging kludge--is this really the way to share global loggers?
var (
	INFO *log.Logger
)

type Ringbuffer struct {
	array [60]int64
	head  int
}

func MakeNew(infolog *log.Logger) *Ringbuffer {
	rb := new(Ringbuffer)
	INFO = infolog
	go rb.advanceWithTimer()
	return rb
}

func (rb *Ringbuffer) advanceWithTimer() {
	ticker := time.Tick(1 * time.Second)
	for now := range ticker {
		currentSecond := now.Second()
		if currentSecond > 59 {
			currentSecond = 0
		}
		rb.head = currentSecond
	}
	// is this a race condition? need a lock here?
	rb.ResetNextVal()
}

func (rb *Ringbuffer) GetVal() int64 {
	return rb.array[rb.head]
}

func (rb *Ringbuffer) GetValAt(val int) int64 {
	return rb.array[val]
}
func (rb *Ringbuffer) GetValAtRelative(vector int) int64 {
	i := rb.head + vector // vector can be negative
	if i > 59 {
		i -= 60
	} else if i < 0 {
		i = 60 + i
	}
	return rb.array[i]
}

func (rb *Ringbuffer) GetPrevVal() int64 {
	i := rb.head
	i--
	if i < 0 {
		i = len(rb.array) - 1
	}
	return rb.array[i]
}

func (rb *Ringbuffer) IncrementHead() int64 {
	rb.array[rb.head] += 1
	return rb.array[rb.head]
}

func (rb *Ringbuffer) ChangeHeadBy(val int64) int64 {
	rb.array[rb.head] += val
	return rb.array[rb.head]
}

func (rb *Ringbuffer) IncrementAt(i int) {
	// this will panic on index out-of-bounds, that's good
	rb.array[i]++
}

func (rb *Ringbuffer) IncrementAtBy(i int, val int64) int64 {
	rb.array[i] += val

	return rb.array[i]
}

func (rb *Ringbuffer) AdvanceHead() {
	rb.head += 1
	if rb.head >= len(rb.array) {
		rb.head = 0
	}
}
func (rb *Ringbuffer) AdvanceHeadBy(delta int) {
	rb.head += delta
	if rb.head >= len(rb.array) {
		rb.head = rb.head - len(rb.array) - 1
	}
}

func (rb *Ringbuffer) ResetNextVal() {
	i := rb.head
	i++
	if i >= len(rb.array) {
		i = 0
	}
	rb.array[i] = 0
}

func (rb *Ringbuffer) Length() int {
	return len(rb.array)
}

func (rb *Ringbuffer) GetArray() []int64 {
	return rb.array[:] // right?
}

func (rb *Ringbuffer) GetMax() int64 {
	var max = int64(0)
	for i := range rb.array {
		if rb.array[i] > max {
			max = rb.array[i]
		}
	}
	return max
}

func (rb *Ringbuffer) SumPrevN(n int) int64 {

	var sum int64 = 0

	for i := 0; i < n; i++ {
		vector := (i * -1) - 1
		sum += rb.GetValAtRelative(vector)
	}
	return sum
}
