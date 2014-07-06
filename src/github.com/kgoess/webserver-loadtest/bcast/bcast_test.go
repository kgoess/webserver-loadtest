package bcast

import "testing"

//import "fmt"


func TestBcastBasic(t *testing.T) {

    var msg interface{}

    testCh := make(chan interface{})

    bcaster := Bcast{ incomingCh: testCh }
    go bcaster.ListenOnIncoming()

    subscriberCh1 := make(chan interface{})
    subscriberCh2 := make(chan interface{})
    subscriberCh3 := make(chan interface{})

    bcaster.Join(subscriberCh1)


    testCh <- "hi mom"
    msg = <-subscriberCh1
    if msg != "hi mom" {
        t.Errorf("expected hi mom, got %v", msg)
    }

    bcaster.Join(subscriberCh2)
    bcaster.Join(subscriberCh3)
    testCh <- "happy birthday"
    if msg = <-subscriberCh1; msg != "happy birthday" {
        t.Errorf("expected happy birthday, got %v", msg)
    }
    if msg = <-subscriberCh2; msg != "happy birthday" {
        t.Errorf("expected happy birthday, got %v", msg)
    }
    if msg = <-subscriberCh3; msg != "happy birthday" {
        t.Errorf("expected happy birthday, got %v", msg)
    }

//    if x := rb.Length(); x != 60 {
//        t.Errorf("length() = %v, want 60", x)
//    }
}

