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

    // if we naively close a channel, the bcaster will crash
    // so need to unjoin
    close(subscriberCh2)
    bcaster.Unjoin(subscriberCh2)
    testCh <- "pity da foo"
    if msg = <-subscriberCh1; msg != "pity da foo" {
        t.Errorf("expected pity da foo, got %v", msg)
    }
    if msg = <-subscriberCh3; msg != "pity da foo" {
        t.Errorf("expected pity da foo, got %v", msg)
    }

    // test the convenience method
    bcaster.UnjoinAndClose(subscriberCh1)
    testCh <- "happy birthday"
    if msg = <-subscriberCh3; msg != "happy birthday" {
        t.Errorf("expected happy birthday, got %v", msg)
    }

}

