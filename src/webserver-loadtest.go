package main

import (
    gc "code.google.com/p/goncurses"
    "log"
    //"io"
     "os"
     "fmt"
    "strconv"
    "net/http"
    "time"
    "flag"

    "math/rand"

)

var (
    TRACE   *log.Logger
    INFO    *log.Logger
    WARNING *log.Logger
    ERROR   *log.Logger
)

const (
    MSG_TYPE_RESULT int = 0
    MSG_TYPE_INFO   int = 1
)

type ncursesMsg struct {
    msgStr string
    currentCount int
    msgType int
}

type currentBars struct {
    cols []int
    failCols []int
}

// See https://groups.google.com/forum/#!topic/golang-nuts/_Twwb5ULStM
// So that defer will run propoerly
// Remember Exit(0) is success, Exit(1) is failure
func main(){
    os.Exit(realMain())
}

func realMain() int {


rand.Seed(42) // Try changing this number!


    var testUrl = flag.String("url", "", "the url you want to beat on")
    var logFile = flag.String("logfile", "./loadtest.log", "path to log file (default loadtest.log)")
    flag.Parse();

    // set up logging
    logWriter, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
    log.SetOutput(logWriter);

    INFO = log.New(logWriter,
            "INFO: ",
            log.Ldate|log.Ltime|log.Lshortfile)
    ERROR = log.New(logWriter,
            "ERROR: ",
            log.Ldate|log.Ltime|log.Lshortfile)
    INFO.Println("beginning run")


    // initialize ncurses
    stdscr, err := gc.Init()
    if err != nil {
        log.Fatal(err)
    }
    defer gc.End()

    // Turn off character echo, hide the cursor and disable input buffering
    gc.Echo(false)
    gc.CBreak(true)
    gc.StartColor()

    // initialize colors
    whiteOnBlack := int16(1)
    gc.InitPair(whiteOnBlack, gc.C_WHITE, gc.C_BLACK)
    greenOnBlack := int16(2)
    gc.InitPair(greenOnBlack, gc.C_GREEN, gc.C_BLACK)
    redOnBlack := int16(3)
    gc.InitPair(redOnBlack, gc.C_RED, gc.C_BLACK)

    // print startup message
    gc.Cursor(0)
    stdscr.Print("Press 'q' to exit")
    stdscr.NoutRefresh()

    msgHeight, msgWidth := 5, 40
    msgY, msgX := 1, 0

    // Create message window 
    // and enable the use of the
    // keypad on it so the arrow keys are available
    var msgWin *gc.Window
    msgWin, err = gc.NewWindow(msgHeight, msgWidth, msgY, msgX)
    if err != nil {
        log.Fatal(err)
    }
    msgWin.Keypad(true)

    // Create the counter window, showing how many goroutines are active
    ctrHeight, ctrWidth := 3, 7
    ctrY := 1
    ctrX := msgWidth + 1
    var workerCountWin *gc.Window
    workerCountWin, err = gc.NewWindow(ctrHeight, ctrWidth, ctrY, ctrX)
    if err != nil {
        log.Fatal(err)
    }


    // Create the bars window, showing the moving display of bars
    barsHeight, barsWidth := 20, 80 // need to size this dynamically, TBD
    barsY := msgHeight + 1
    barsX := 1
    var barsWin *gc.Window
    barsWin, err = gc.NewWindow(barsHeight, barsWidth, barsY, barsX)
    if err != nil {
        log.Fatal(err)
    }

    // Clear the section of screen where the box is currently located so
    // that it is blanked by calling Erase on the window and refreshing it
    // so that the changes are sent to the virtual screen but not actually
    // output to the terminal
    //msgWin.ColorOn(whiteOnBlack)
    msgWin.Erase()
    msgWin.NoutRefresh()
    msgWin.MoveWindow(msgY, msgX)
    msgWin.Box(0, 0)
    msgWin.NoutRefresh()

    //workerCountWin.ColorOn(whiteOnBlack)
    workerCountWin.Erase()
    workerCountWin.NoutRefresh()
    workerCountWin.Box(0, 0)
    workerCountWin.NoutRefresh()

    //barsWin.ColorOn(whiteOnBlack)
    barsWin.Erase()
    barsWin.NoutRefresh()
    barsWin.Box(0, 0)
    barsWin.NoutRefresh()

    // Update will flush only the characters which have changed between the
    // physical screen and the virtual screen, minimizing the number of
    // characters which must be sent
    gc.Update()

    // create our various channels
    infoMsgsCh            := make(chan ncursesMsg)
    exitCh                := make(chan int)
    changeNumRequestersCh := make(chan int)
    reqMadeOnSecCh        := make(chan int)
    failsOnSecCh          := make(chan int)
    barsToDrawCh          := make(chan currentBars)

    go windowRunloop(infoMsgsCh, exitCh, changeNumRequestersCh, msgWin)
    go requesterController(infoMsgsCh, changeNumRequestersCh, reqMadeOnSecCh, failsOnSecCh, *testUrl)
    go barsController(reqMadeOnSecCh, failsOnSecCh, barsToDrawCh)

    var exitStatus int

    main:
    for {
        select {
        case msg := <-infoMsgsCh:
            var row int
            if msg.msgType == MSG_TYPE_RESULT {
                row = 1
            }else if msg.msgType == MSG_TYPE_INFO {
                row = 2
            }else{
                row = 3
            }
            msgWin.MovePrint(row, 1, fmt.Sprintf("%-40s", msg.msgStr))
            msgWin.Box(0, 0)
            msgWin.NoutRefresh()
            if msg.currentCount >= 0 {
                workerCountWin.MovePrint(1, 1, fmt.Sprintf("%5d", msg.currentCount))
                workerCountWin.NoutRefresh()
            }
            gc.Update()
        case msg := <-barsToDrawCh:
            //barsWin.Erase()
INFO.Println("got a barsToDrawCh msg ", msg)
            edibleCopy := make([]int, len(msg.cols))
            copy(edibleCopy, msg.cols)
            startI := len(edibleCopy)-barsWidth
            if startI < 0{
                startI = 0
            }
            currentSec := time.Now().Second()
            for row := 0; row < len(edibleCopy); row++ {
                for col := range edibleCopy[ startI:len(edibleCopy) ]{
                    if edibleCopy[col] > 0 {
                        turnOffColor := int16(0)
                        currChar := "="
                        if msg.failCols[col] > row { 
                            barsWin.ColorOff(whiteOnBlack)
                            barsWin.ColorOn(redOnBlack)
                            currChar = "x"
                            turnOffColor = redOnBlack

                        }else if col == currentSec {
                            barsWin.ColorOff(whiteOnBlack)
                            barsWin.ColorOn(greenOnBlack)
                            turnOffColor = greenOnBlack
                        }
                        barsWin.MovePrint(barsHeight-2-row, col+1, currChar)
                        if turnOffColor != 0  {
                            barsWin.ColorOff(turnOffColor)
                            barsWin.ColorOn(whiteOnBlack)
                        }
                        edibleCopy[col]--
                    }else{
                        // TBD just erase the whole screen at the beginnig
                        barsWin.MovePrint(barsHeight-2-row, col+1, " ") 
                    }
                }
            }
            barsWin.NoutRefresh()
            gc.Update()
        case exitStatus = <-exitCh:
            break main
        }
    }

    msgWin.Delete()
    gc.End()
    INFO.Println("exiting with status ", exitStatus)
    return exitStatus
}


func windowRunloop(infoMsgsCh chan ncursesMsg, exitCh chan int, changeNumRequestersCh chan int, win *gc.Window){
    threadCount := 0
    for {
        switch win.GetChar() {
            case 'q':
                exitCh <- 0
            case 's', '+', '=', gc.KEY_UP:
                threadCount++
                increaseThreads(infoMsgsCh, changeNumRequestersCh, win, threadCount);
            case '-', gc.KEY_DOWN:
                threadCount--
                decreaseThreads(infoMsgsCh, changeNumRequestersCh, win, threadCount);
        }
    }
}

func increaseThreads(infoMsgsCh chan ncursesMsg, changeNumRequestersCh chan int, win *gc.Window, threadCount int ) {
    INFO.Println("increasing threads to ", threadCount)
    infoMsgsCh <- ncursesMsg{ "increasing threads", threadCount, MSG_TYPE_INFO }
    changeNumRequestersCh <- 1
}

func decreaseThreads(infoMsgsCh chan ncursesMsg, changeNumRequestersCh chan int, win *gc.Window, threadCount int ) {
    INFO.Println("decreasing threads to ", threadCount)
    infoMsgsCh <- ncursesMsg{ "decreasing threads", threadCount, MSG_TYPE_INFO}
    changeNumRequestersCh <- -1
}

func requesterController(infoMsgsCh chan ncursesMsg, changeNumRequestersCh chan int, reqMadeOnSecCh chan int, failsOnSecCh chan int, testUrl string){


    //var chans = []chan int
    // this creates a slice associated with an underlying array
    chans := make([]chan int, 0)

    for {
        select {
        case upOrDown := <-changeNumRequestersCh:
            if upOrDown == 1 {
                shutdownChan := make(chan int)
                chans = append(chans, shutdownChan)
                chanId := len(chans)-1
                go requester(infoMsgsCh, shutdownChan, chanId, reqMadeOnSecCh, failsOnSecCh, testUrl)
            }else if upOrDown == -1 && len(chans) > 0{
                //send shutdown message
                chans[len(chans)-1]  <-1
                // throw away that channel
                chans = chans[0:len(chans)-1]
            }else{
                INFO.Println("ignoring decrease--there aren't any channels")
            }
        }
    }
}

func requester(infoMsgsCh chan ncursesMsg, shutdownChan chan int, id int, reqMadeOnSecCh chan int, failsOnSecCh chan int, testUrl string) {

    var i int64 = 0
    var shutdownNow bool = false

    for {
        select {
            case _ = <-shutdownChan:
                INFO.Println("shutting down #", id);
                shutdownNow = true
            default:
                i++
thisUrl := testUrl
if rand.Intn(5) > 3 {
thisUrl = testUrl + "blearg"
}
                hitId := strconv.FormatInt(int64(id), 10) + ":" + strconv.FormatInt(i, 10)
//                 resp, err := http.Get(testUrl + "?" + hitId) // TBD make that appending conditional
resp, err := http.Get(thisUrl + "?" + hitId) // TBD make that appending conditional
                sec := time.Now().Second()
                reqMadeOnSecCh <-sec
                if err == nil && resp.StatusCode == 200 {
                    INFO.Println(id, "/", i,  " fetch ok ")
                    infoMsgsCh <- ncursesMsg{ "request ok " + hitId, -1, MSG_TYPE_RESULT }
                }else if err != nil {
                    ERROR.Println("http get failed: ", err)
                    infoMsgsCh <- ncursesMsg{ "request fail " + hitId, -1, MSG_TYPE_RESULT }
                    failsOnSecCh <- sec
                }else{
                    ERROR.Println("http get failed: ", resp.Status)
                    infoMsgsCh <- ncursesMsg{ "request fail " + hitId, -1, MSG_TYPE_RESULT }
                    failsOnSecCh <- sec
                }


                time.Sleep(1000 * time.Millisecond)
        }
        if shutdownNow {
            return
        }
    }
}

func barsController(reqMadeOnSecCh chan int, failsOnSecCh chan int, barsToDrawCh chan currentBars){
    var secondsToStore = 60
    var requestsForSecond [60]int  // one column for each clock second
    var failsForSecond [60]int  // one column for each clock second
    for i := range requestsForSecond{
        requestsForSecond[i] = 0
    }
    timeToRedraw := make( chan bool)
    go func (timeToRedraw chan bool) {
        for {
            time.Sleep(1000 * time.Millisecond)
            timeToRedraw <- true
        }
    }(timeToRedraw)

    for {
        select {
        case msg := <-reqMadeOnSecCh:
            requestsForSecond[msg]++
        case msg := <-failsOnSecCh:
            failsForSecond[msg]++
        case <-timeToRedraw:
            // zero out the *next* second, aka 60 seconds *ago* ;-)1
            nextSec := time.Now().Second() + 1
            if nextSec >= secondsToStore {
                nextSec = 0
            }
            requestsForSecond[nextSec] = 0
            failsForSecond[nextSec] = 0
            barsToDrawCh <- currentBars{ requestsForSecond[:], failsForSecond[:] }
        }
    }
}

/*
259 up
258 down
61 plus
43 shift plus
45 minus
*/

