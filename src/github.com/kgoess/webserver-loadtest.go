package main

import (
	gc "code.google.com/p/goncurses"
	"log"
	//"io"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

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
	msgStr       string
	currentCount int
	msgType      int
}

type currentBars struct {
	cols     []int
	failCols []int
}

type colorsDefined struct {
	whiteOnBlack int16
	greenOnBlack int16
	redOnBlack   int16
}

var testUrl = flag.String("url", "", "the url you want to beat on")
var logFile = flag.String("logfile", "./loadtest.log", "path to log file (default loadtest.log)")
var introduceRandomFails = flag.Int("random-fails", 0, "introduce x/10 random failures")

// Remember Exit(0) is success, Exit(1) is failure
func main() {
	flag.Parse()
	if len(*testUrl) == 0 {
		flag.Usage()
		os.Exit(1)
	}
	rand.Seed(time.Now().Unix())

	// set up logging
	logWriter, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		println(err)
		os.Exit(1)
	}
	log.SetOutput(logWriter)

	INFO = log.New(logWriter,
		"INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)
	ERROR = log.New(logWriter,
		"ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)
	INFO.Println("beginning run")

	os.Exit(realMain())
}

// Why realMain? See https://groups.google.com/forum/#!topic/golang-nuts/_Twwb5ULStM
// So that defer will run propoerly
func realMain() int {

	// initialize ncurses
	stdscr, colors := initializeNcurses()

	// draw the stuff on the screen
	msgWin, workerCountWin, durWin, reqSecWin, barsWin := drawDisplay(stdscr)

	// create our various channels
	infoMsgsCh := make(chan ncursesMsg)
	exitCh := make(chan int)
	changeNumRequestersCh := make(chan int)
	reqMadeOnSecCh := make(chan int)
	failsOnSecCh := make(chan int)
	durationCh := make(chan int64)
	durationDisplayCh := make(chan string)
	reqSecCh := make(chan int64)
	reqSecDisplayCh := make(chan string)
	barsToDrawCh := make(chan currentBars)

	// start all the worker goroutines
	go windowRunloop(infoMsgsCh, exitCh, changeNumRequestersCh, msgWin)
	go requesterController(infoMsgsCh, changeNumRequestersCh, reqMadeOnSecCh, failsOnSecCh, durationCh, *testUrl, *introduceRandomFails)
	go barsController(reqMadeOnSecCh, failsOnSecCh, barsToDrawCh)
	go statsWinsController(durationCh, durationDisplayCh, reqSecCh, reqSecDisplayCh)

	var exitStatus int

	// This is the main loop controlling the ncurses display. Since ncurses
	// wasn't designed with concurrency in mind, only one goroutine should
	// write to a window, so I'm putting all the window writing in here.
main:
	for {
		select {
		case msg := <-infoMsgsCh:
			updateMsgWin(msg, msgWin, workerCountWin)
		case msg := <-durationDisplayCh:
			// that %7s should really be determined from durWidth
			durWin.MovePrint(1, 1, fmt.Sprintf("%7s", msg))
			durWin.NoutRefresh()
		case msg := <-reqSecDisplayCh:
			reqSecWin.MovePrint(1, 1, fmt.Sprintf("%7s", msg))
			reqSecWin.NoutRefresh()
		case msg := <-barsToDrawCh:
			updateBarsWin(msg, barsWin, *colors)
		case exitStatus = <-exitCh:
			break main
		}

		gc.Update()
	}

	msgWin.Delete()
	gc.End()
	INFO.Println("exiting with status ", exitStatus)
	return exitStatus
}

func initializeNcurses() (stdscr *gc.Window, colors *colorsDefined) {

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

	// Set the cursor visibility.
	// Options are: 0 (invisible/hidden), 1 (normal) and 2 (extra-visible)
	gc.Cursor(0)

	colors = &colorsDefined{whiteOnBlack, greenOnBlack, redOnBlack}

	return
}

func drawDisplay(
	stdscr *gc.Window,
) (
	msgWin *gc.Window,
	workerCountWin *gc.Window,
	durWin *gc.Window,
	reqSecWin *gc.Window,
	barsWin *gc.Window,
) {

	// print startup message
	stdscr.Print("Press 'q' to exit")
	stdscr.NoutRefresh()

	// Create message window
	// and enable the use of the
	// keypad on it so the arrow keys are available
	msgHeight, msgWidth := 5, 40
	msgY, msgX := 1, 0
	msgWin = createWindow(msgHeight, msgWidth, msgY, msgX)
	msgWin.Keypad(true)
	msgWin.Box(0, 0)
	msgWin.NoutRefresh()

	// Create the counter window, showing how many goroutines are active
	ctrHeight, ctrWidth := 3, 7
	ctrY := 2
	ctrX := msgWidth + 1
	stdscr.MovePrint(1, ctrX+1, "thrds")
	stdscr.NoutRefresh()
	workerCountWin = createWindow(ctrHeight, ctrWidth, ctrY, ctrX)
	workerCountWin.Box(0, 0)
	workerCountWin.NoutRefresh()

	// Create the avg duration window, showing 5 second moving average
	durHeight, durWidth := 3, 9
	durY := 2
	durX := ctrX + ctrWidth + 1
	stdscr.MovePrint(1, durX+1, "av dur")
	stdscr.NoutRefresh()
	durWin = createWindow(durHeight, durWidth, durY, durX)
	durWin.Box(0, 0)
	durWin.NoutRefresh()

	// Create the requests/sec window,
	reqSecHeight, reqSecWidth := 3, 9
	reqSecY := 2
	reqSecX := durX + durWidth + 1
	stdscr.MovePrint(1, reqSecX+1, "req/s")
	stdscr.NoutRefresh()
	reqSecWin = createWindow(reqSecHeight, reqSecWidth, reqSecY, reqSecX)
	reqSecWin.Box(0, 0)
	reqSecWin.NoutRefresh()

	// Create the bars window, showing the moving display of bars
	barsHeight, barsWidth := 25, 80 // need to size this dynamically, TBD
	barsY := msgHeight + 1
	barsX := 1
	barsWin = createWindow(barsHeight, barsWidth, barsY, barsX)
	barsWin.Box(0, 0)
	barsWin.NoutRefresh()

	// Update will flush only the characters which have changed between the
	// physical screen and the virtual screen, minimizing the number of
	// characters which must be sent
	gc.Update()

	return
}

func createWindow(height int, width int, y int, x int) (win *gc.Window) {
	win, err := gc.NewWindow(height, width, y, x)
	if err != nil {
		log.Fatal(err)
	}
	return
}

func updateMsgWin(msg ncursesMsg, msgWin *gc.Window, workerCountWin *gc.Window) {
	var row int
	if msg.msgType == MSG_TYPE_RESULT {
		row = 1
	} else if msg.msgType == MSG_TYPE_INFO {
		row = 2
	} else {
		row = 3
	}
	msgWin.MovePrint(row, 1, fmt.Sprintf("%-40s", msg.msgStr))
	msgWin.Box(0, 0)
	msgWin.NoutRefresh()
	if msg.currentCount >= 0 {
		workerCountWin.MovePrint(1, 1, fmt.Sprintf("%5d", msg.currentCount))
		workerCountWin.NoutRefresh()
	}
}
func updateBarsWin(msg currentBars, barsWin *gc.Window, colors colorsDefined) {

	whiteOnBlack := colors.whiteOnBlack
	redOnBlack := colors.redOnBlack
	greenOnBlack := colors.greenOnBlack
	barsWin.Erase()
	barsWin.Box(0, 0)
	edibleCopy := make([]int, len(msg.cols))
	copy(edibleCopy, msg.cols)
	barsHeight, barsWidth := barsWin.MaxYX()
	startI := len(edibleCopy) - barsWidth
	if startI < 0 {
		startI = 0
	}
	currentSec := time.Now().Second()
	for row := 0; row < barsHeight-2; row++ {
		for col := range edibleCopy[startI:len(edibleCopy)] {
			if edibleCopy[col] > 0 {
				turnOffColor := int16(0)
				currChar := "="
				if msg.failCols[col] > row {
					barsWin.ColorOff(whiteOnBlack)
					barsWin.ColorOn(redOnBlack)
					currChar = "x"
					turnOffColor = redOnBlack

				} else if col == currentSec {
					barsWin.ColorOff(whiteOnBlack)
					barsWin.ColorOn(greenOnBlack)
					turnOffColor = greenOnBlack
				}
				barsWin.MovePrint(barsHeight-2-row, col+1, currChar)
				if turnOffColor != 0 {
					barsWin.ColorOff(turnOffColor)
					barsWin.ColorOn(whiteOnBlack)
				}
				edibleCopy[col]--
			}
		}
	}
	barsWin.NoutRefresh()
}

func windowRunloop(
	infoMsgsCh chan ncursesMsg,
	exitCh chan int,
	changeNumRequestersCh chan int,
	win *gc.Window,
) {
	threadCount := 0
	for {
		switch win.GetChar() {
		case 'q':
			exitCh <- 0
		case 's', '+', '=', gc.KEY_UP:
			threadCount++
			increaseThreads(infoMsgsCh, changeNumRequestersCh, win, threadCount)
		case '-', gc.KEY_DOWN:
			threadCount--
			decreaseThreads(infoMsgsCh, changeNumRequestersCh, win, threadCount)
		}
	}
}

func increaseThreads(
	infoMsgsCh chan ncursesMsg,
	changeNumRequestersCh chan int,
	win *gc.Window,
	threadCount int,
) {
	INFO.Println("increasing threads to ", threadCount)
	infoMsgsCh <- ncursesMsg{"increasing threads", threadCount, MSG_TYPE_INFO}
	changeNumRequestersCh <- 1
}

func decreaseThreads(
	infoMsgsCh chan ncursesMsg,
	changeNumRequestersCh chan int,
	win *gc.Window,
	threadCount int,
) {
	INFO.Println("decreasing threads to ", threadCount)
	infoMsgsCh <- ncursesMsg{"decreasing threads", threadCount, MSG_TYPE_INFO}
	changeNumRequestersCh <- -1
}

func requesterController(
	infoMsgsCh chan ncursesMsg,
	changeNumRequestersCh chan int,
	reqMadeOnSecCh chan int,
	failsOnSecCh chan int,
	durationCh chan int64,
	testUrl string,
	introduceRandomFails int,
) {

	//var chans = []chan int
	// this creates a slice associated with an underlying array
	chans := make([]chan int, 0)

	for {
		select {
		case upOrDown := <-changeNumRequestersCh:
			if upOrDown == 1 {
				shutdownChan := make(chan int)
				chans = append(chans, shutdownChan)
				chanId := len(chans) - 1
				go requester(infoMsgsCh, shutdownChan, chanId, reqMadeOnSecCh, failsOnSecCh, durationCh, testUrl, introduceRandomFails)
			} else if upOrDown == -1 && len(chans) > 0 {
				//send shutdown message
				chans[len(chans)-1] <- 1
				// throw away that channel
				chans = chans[0 : len(chans)-1]
			} else {
				INFO.Println("ignoring decrease--there aren't any channels")
			}
		}
	}
}

func requester(
	infoMsgsCh chan ncursesMsg,
	shutdownChan chan int,
	id int,
	reqMadeOnSecCh chan int,
	failsOnSecCh chan int,
	durationCh chan int64,
	testUrl string,
	introduceRandomFails int,
) {

	var i int64 = 0
	var shutdownNow bool = false

	for {
		select {
		case _ = <-shutdownChan:
			INFO.Println("shutting down #", id)
			shutdownNow = true
		default:
			i++
			thisUrl := testUrl
			if introduceRandomFails > 0 && rand.Intn(10) < introduceRandomFails {
				thisUrlStruct, _ := url.Parse(thisUrl)
				thisUrlStruct.Path = "-artificial-random-failure-" + thisUrlStruct.Path
				thisUrl = thisUrlStruct.String()
			}
			hitId := strconv.FormatInt(int64(id), 10) + ":" + strconv.FormatInt(i, 10)
			t0 := time.Now()
			resp, err := http.Get(thisUrl + "?" + hitId) // TBD make that appending conditional
			t1 := time.Now()
			resp.Body.Close()
			durationCh <- int64(t1.Sub(t0) / time.Millisecond)
			nowSec := time.Now().Second()
			reqMadeOnSecCh <- nowSec
			if err == nil && resp.StatusCode == 200 {
				INFO.Println(id, "/", i, " fetch ok ")
				infoMsgsCh <- ncursesMsg{"request ok " + hitId, -1, MSG_TYPE_RESULT}
			} else if err != nil {
				ERROR.Println("http get failed: ", err)
				infoMsgsCh <- ncursesMsg{"request fail " + hitId, -1, MSG_TYPE_RESULT}
				failsOnSecCh <- nowSec
			} else {
				ERROR.Println("http get failed: ", resp.Status)
				infoMsgsCh <- ncursesMsg{"request fail " + hitId, -1, MSG_TYPE_RESULT}
				failsOnSecCh <- nowSec
			}

			// just for development
			time.Sleep(1000 * time.Millisecond)
		}
		if shutdownNow {
			return
		}
	}
}

func barsController(
	reqMadeOnSecCh chan int,
	failsOnSecCh chan int,
	barsToDrawCh chan currentBars,
) {
	var secondsToStore = 60
	var requestsForSecond [60]int // one column for each clock second
	var failsForSecond [60]int    // one column for each clock second
	for i := range requestsForSecond {
		requestsForSecond[i] = 0
	}
	timeToRedraw := make(chan bool)
	go func(timeToRedraw chan bool) {
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
			barsToDrawCh <- currentBars{requestsForSecond[:], failsForSecond[:]}
		}
	}
}

func statsWinsController(
	durationCh chan int64,
	durationDisplayCh chan string,
	reqSecCh chan int64,
	reqSecDisplayCh chan string,
) {
	var totalDurForSecond [60]int64 // total durations for each clock second
	var countForSecond [60]int64    // how many received per second
	//var averagesArr [60]float64
	window := 3
	//var averages []float64 = averagesArr[0:window]

	timeToRedraw := make(chan bool)
	go func(timeToRedraw chan bool) {
		for {
			time.Sleep(1000 * time.Millisecond)
			timeToRedraw <- true
		}
	}(timeToRedraw)

	for {
		select {
		case dur := <-durationCh:
			currSec := time.Now().Second()
			totalDurForSecond[currSec] += dur
			countForSecond[currSec]++
		//case <-time.After(1 * time.Second):
		case <-timeToRedraw:
			currSec := time.Now().Second()

			var windowDur int64
			var windowCount int64
			for i := currSec - window; i < currSec; i++ {
				index := i
				if index < 0 {
					index += 60
				}
				windowDur += totalDurForSecond[index]
				windowCount += countForSecond[index]
			}
			if windowCount > 0 {
				INFO.Println("windowDur is ", windowDur, " and windowCount is ", windowCount, " so avg is ", float64(windowDur)/float64(windowCount))
				durationDisplayCh <- fmt.Sprintf("%4.2f", float64(windowDur)/float64(windowCount))
			} else {
				durationDisplayCh <- "0"
			}
			countForSecIndex := currSec - 1
			if countForSecIndex < 0 {
				countForSecIndex = 59
			}
			reqSecDisplayCh <- fmt.Sprintf("%d", countForSecond[countForSecIndex])
			//reqSecDisplayCh <- fmt.Sprintf("%d", currSec)
			nextSec := time.Now().Second() + 1
			if nextSec >= 60 {
				nextSec = 0
			}
			totalDurForSecond[nextSec] = 0
			countForSecond[nextSec] = 0
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