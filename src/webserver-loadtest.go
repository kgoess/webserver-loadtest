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
}

// See https://groups.google.com/forum/#!topic/golang-nuts/_Twwb5ULStM
// So that defer will run propoerly
// Remember Exit(0) is success, Exit(1) is failure
func main(){
    os.Exit(realMain())
}

func realMain() int {

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

    whiteOnBlack := int16(1)
    gc.InitPair(whiteOnBlack, gc.C_WHITE, gc.C_BLACK)
    greenOnBlack := int16(2)
    gc.InitPair(greenOnBlack, gc.C_GREEN, gc.C_BLACK)


    // print startup message
    gc.Cursor(0)
    stdscr.Print("Press 'q' to exit")
    stdscr.NoutRefresh()

    // Determine the center of the screen and offset those coordinates by
    // half of the window size we are about to create. These coordinates will
    // be used to move our window around the screen
    //rows, cols := stdscr.MaxYX()
    height, width := 5, 40
    //y, x := (rows-height)/2, (cols-width)/2
    y, x := 1, 0

    // Create control window 
    // and enable the use of the
    // keypad on it so the arrow keys are available
    var msgWin *gc.Window
    msgWin, err = gc.NewWindow(height, width, y, x)
    if err != nil {
        log.Fatal(err)
    }
    msgWin.Keypad(true)

    ctrHeight, ctrWidth := 3, 7
    ctrY := 1
    ctrX := width + 1
    var workerCountWin *gc.Window
    workerCountWin, err = gc.NewWindow(ctrHeight, ctrWidth, ctrY, ctrX)
    if err != nil {
        log.Fatal(err)
    }


    barsHeight, barsWidth := 20, 80 // need to size this dynamically, TBD
    barsY := height + 1
    barsX := 1
    var barsWin *gc.Window
    barsWin, err = gc.NewWindow(barsHeight, barsWidth, barsY, barsX)
    if err != nil {
        log.Fatal(err)
    }

    // Clear the section of screen where the box is currently located so
    // that it is blanked by calling Erase on the window and refreshing it
    // so that the chances are sent to the virtual screen but not actually
    // output to the terminal
    //msgWin.ColorOn(whiteOnBlack)
    msgWin.Erase()
    msgWin.NoutRefresh()
    msgWin.MoveWindow(y, x)
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

    infoMsgsCh := make(chan ncursesMsg)
    exitCh := make(chan int)
    requesterChan := make(chan int)
    toBarsControl := make(chan int)
    drawBars := make(chan currentBars)

    go windowRunloop(infoMsgsCh, exitCh, requesterChan, msgWin)
    go requesterController(infoMsgsCh, requesterChan, toBarsControl, *testUrl)
    go barsController(toBarsControl, drawBars)

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
        case msg := <-drawBars:
            //barsWin.Erase()
INFO.Println("got a drawBars msg ", msg)
            edibleCopy := make([]int, len(msg.cols))
            copy(edibleCopy, msg.cols)
            startI := len(edibleCopy)-barsWidth
            if startI < 0{
                startI = 0
            }
            _, _, sec := time.Now().Clock()
            sec--
            for row := 0; row < len(edibleCopy); row++ {
                for col := range edibleCopy[ startI:len(edibleCopy) ]{
                    if edibleCopy[col] > 0 {
                        if col == sec {
                            barsWin.ColorOff(whiteOnBlack)
                            barsWin.ColorOn(greenOnBlack)
                        }
                        barsWin.MovePrint(barsHeight-2-row, col+1, "=")
                        if col == sec {
                            barsWin.ColorOff(greenOnBlack)
                            barsWin.ColorOn(whiteOnBlack)
                        }
                        edibleCopy[col]--
                    }else{
                        barsWin.MovePrint(barsHeight-2-row, col+1, " ") // TBD just erase the whole screen at the beginnig
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


func windowRunloop(infoMsgsCh chan ncursesMsg, exitCh chan int, requesterChan chan int, win *gc.Window){
    threadCount := 0
    for {
        switch win.GetChar() {
            case 'q':
                exitCh <- 0
            case 's', '+', '=', gc.KEY_UP:
                threadCount++
                increaseThreads(infoMsgsCh, requesterChan, win, threadCount);
            case '-', gc.KEY_DOWN:
                threadCount--
                decreaseThreads(infoMsgsCh, requesterChan, win, threadCount);
        }
    }
}

func increaseThreads(infoMsgsCh chan ncursesMsg, requesterChan chan int, win *gc.Window, threadCount int ) {
    INFO.Println("increasing threads to ", threadCount)
    infoMsgsCh <- ncursesMsg{ "increasing threads", threadCount, MSG_TYPE_INFO }
    requesterChan <- 1
}

func decreaseThreads(infoMsgsCh chan ncursesMsg, requesterChan chan int, win *gc.Window, threadCount int ) {
    INFO.Println("decreasing threads to ", threadCount)
    infoMsgsCh <- ncursesMsg{ "decreasing threads", threadCount, MSG_TYPE_INFO}
    requesterChan <- -1
}

func requesterController(infoMsgsCh chan ncursesMsg, requesterChan chan int, toBarsControl chan int, testUrl string){


    //var chans = []chan int
    // this creates a slice associated with an underlying array
    chans := make([]chan int, 0)

    for {
        select {
        case upOrDown := <-requesterChan:
            if upOrDown == 1 {
                shutdownChan := make(chan int)
                chans = append(chans, shutdownChan)
                chanId := len(chans)-1
                go requester(infoMsgsCh, shutdownChan, chanId, toBarsControl, testUrl)
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

func requester(infoMsgsCh chan ncursesMsg, shutdownChan chan int, id int, toBarsControl chan int, testUrl string) {

    var i int64 = 0
    var shutdownNow bool = false

    for {
        select {
            case _ = <-shutdownChan:
                INFO.Println("shutting down #", id);
                shutdownNow = true
            default:
                i++
                hitId := strconv.FormatInt(int64(id), 10) + ":" + strconv.FormatInt(i, 10)
                 _, err := http.Get(testUrl + "?" + hitId) // TBD make that appending conditional
                if err == nil {
                    INFO.Println(id, "/", i,  " fetch ok ", err)
                    infoMsgsCh <- ncursesMsg{ "request ok " + hitId, -1, MSG_TYPE_RESULT }
                }else{
                    ERROR.Println("http get failed: ", err)
                    infoMsgsCh <- ncursesMsg{ "request fail " + hitId, -1, MSG_TYPE_RESULT }
                }

                _, _, sec := time.Now().Clock()
                toBarsControl <-sec

                time.Sleep(1000 * time.Millisecond)
        }
        if shutdownNow {
            return
        }
    }
}

func barsController(toBarsControl chan int, drawBars chan currentBars){
    var secondsToStore = 60
    var requestsForSecond [60]int  // one column for each clock second
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
        case msg := <-toBarsControl:
            requestsForSecond[msg]++
        case <-timeToRedraw:
            _, _, sec := time.Now().Clock()
            sec-- // Clock goes 1 to 60, wtf?
            sec++ // looking at the *next* second, aka 60 seconds *ago* ;-)
            if sec >= secondsToStore {
                sec = 0
            }
            requestsForSecond[sec] = 0
            drawBars <- currentBars{ requestsForSecond[:] }
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

