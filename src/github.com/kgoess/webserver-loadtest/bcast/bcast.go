package bcast

import "log" 
import "fmt"


// debugging kludge--is this really the way to share global loggers?
var (
	INFO *log.Logger
)


type Bcast struct {
    incomingCh chan interface{} // so it'll take anything
    subscribers []chan interface{}
}

// I don't feel like I'm doing constructors right
func MakeNew (infoLog *log.Logger, incomingCh chan interface{}) *Bcast {

    bcaster := new(Bcast)
    bcaster.incomingCh = incomingCh

    go bcaster.ListenOnIncoming()

    return bcaster
}

func (b *Bcast) ListenOnIncoming() {
    for {
        select {
        case msg := <-b.incomingCh:
            for _, forwardToCh := range b.subscribers{
                forwardToCh <- msg
            }
        }
    }
}

func (b *Bcast) Join (listenerCh chan interface{}) {

    // Does this need to be locked?
    // I think not
    // read http://blog.golang.org/slices
    b.subscribers = append(b.subscribers, listenerCh)

    fmt.Println(b.subscribers)
}
