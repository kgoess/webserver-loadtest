package ringbuffer

import "testing"

//import "fmt"


func TestRingbufferBasic(t *testing.T) {
	rb := Ringbuffer{}

	if x := rb.Length(); x != 60 {
		t.Errorf("length() = %v, want 60", x)
	}

	firstIncr := int64(4)
	expectedTot := firstIncr
	rb.ChangeHeadBy(firstIncr)
	if x := rb.GetVal(); x != expectedTot {
		t.Errorf("ChangeHeadBy(%v) = %v, want %v", firstIncr, x, expectedTot)
	}

	secondIncr := int64(7)
	expectedTot += secondIncr
	rb.ChangeHeadBy(secondIncr)
	if x := rb.GetVal(); x != expectedTot {
		t.Errorf("ChangeHeadBy(%v) = %v, want %v", secondIncr, x, expectedTot)
	}

	// next value isn't used yet, s/b 0
	rb.AdvanceHead()
	if x := rb.GetVal(); x != 0 {
		t.Errorf("AdvanceHead() = %v, want 0", x)
	}

	// test vanilla IncrementHead
	rb.IncrementHead()
	rb.IncrementHead()
	if x := rb.GetVal(); x != 2 {
		t.Errorf("IncrementHead() = %v, want 2", x)
	}

	// wrap all the way around, we should be back at slot #0
	rb.AdvanceHeadBy(rb.Length())
	if x := rb.GetVal(); x != expectedTot {
		t.Errorf("AdvanceHeadBy(%v) = %v, want %v", rb.Length(), x, expectedTot)
	}

	// make sure AdvanceHead wraps right
	rb.AdvanceHeadBy(rb.Length() - 1)
	rb.AdvanceHead()
	if x := rb.GetVal(); x != expectedTot {
		t.Errorf("AdvanceHead() = %v, want %v", x, expectedTot)
	}
}

func TestRingbufferAtFunctions( t *testing.T){
	rb := Ringbuffer{}

	rb.IncrementAt(2)
	rb.IncrementAt(2)
	rb.IncrementAt(2)

	rb.IncrementAtBy(3,7)
	rb.IncrementAtBy(3,7)

	rb.AdvanceHead() // to #1
	if x := rb.GetVal(); x != 0 {
		t.Errorf("value at pos 1 s/b 0, got %v", x)
	}

	rb.AdvanceHead() // to #2
	if x := rb.GetVal(); x != 1+1+1 {
		t.Errorf("value at pos 2 s/b 1+1+1=3, got %v", x)
	}

	rb.AdvanceHead() // to #3
	if x := rb.GetVal(); x != 14 {
		t.Errorf("value at pos 3 s/b 7+7=14, got %v", x)
	}

}

/*

make vs. new:
    fmt.Printf("%T  %v\n", new([10]int), new([10]int))
    fmt.Printf("%T  %v\n", make([]int, 10), make([]int, 10))

output:
    *[10]int  &[0 0 0 0 0 0 0 0 0 0]
    []int  [0 0 0 0 0 0 0 0 0 0]

http://stackoverflow.com/questions/8539551/dynamically-initialize-array-size-in-go

*/
