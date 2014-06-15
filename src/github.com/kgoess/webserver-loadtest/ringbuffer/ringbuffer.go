package ringbuffer

type Ringbuffer struct {
	array [60]int64
	head  int
}

func (rb *Ringbuffer) GetVal() int64 {
	return rb.array[rb.head]
}

func (rb *Ringbuffer) GetValAt(val int) int64 {
	return rb.array[val]
}

func (rb *Ringbuffer) GetPrevVal() int64 {
	i := rb.head
	i--
	if i < 0{
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
	rb.array[i] ++
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
	if i >= len(rb.array){
		i = 0
	}
	rb.array[i]= 0
}

func (rb *Ringbuffer) Length() int {
	return len(rb.array)
}

