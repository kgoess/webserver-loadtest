package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"encoding/json"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
	//"io"
	bcast "github.com/kgoess/webserver-loadtest/bcast"
	gc "code.google.com/p/goncurses"
	rb "github.com/kgoess/webserver-loadtest/ringbuffer"
	slave "github.com/kgoess/webserver-loadtest/slave"
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

type bytesPerSecMsg struct {
	bytes         int64
	duration      time.Duration
	receivedOnSec int
}

type SecondStats struct {
	Second int  //redundant, since the key will be the second, maybe we won't need it
	ReqsMade int
	Bytes int64
	Duration time.Duration
}

type currentBars struct {
	cols     []int64
	failCols []int64
	max      int64
}

type colorsDefined struct {
	whiteOnBlack int16
	greenOnBlack int16
	redOnBlack   int16
}

var testUrl = flag.String("url", "", "the url you want to beat on")
var logFile = flag.String("logfile", "./loadtest.log", "path to log file (default loadtest.log)")
var listen = flag.Int("listen", 0, "listen as a client for controller commands on this port")
var introduceRandomFails = flag.Int("random-fails", 0, "introduce x/10 random failures")

var slaveList slave.Slaves

// Remember Exit(0) is success, Exit(1) is failure
func main() {
	flag.Var(&slaveList, "control", "list of ip:port addresses to control")
	flag.Parse()
	if len(*testUrl) == 0 {
		flag.Usage()
		os.Exit(1)
	}
	if len(slaveList) > 0 && *listen != 0 {
		fmt.Fprintf(os.Stderr, "You can't have both --listen and --control flags")
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

	TRACE = log.New(ioutil.Discard,
		"TRACE: ",
		log.Ldate|log.Ltime|log.Lshortfile)
	INFO = log.New(logWriter,
		"INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)
	ERROR = log.New(logWriter,
		"ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)
	INFO.Println("beginning run")

	os.Exit(realMain())
}

type resetScreenFn func()

// Why realMain? See https://groups.google.com/forum/#!topic/golang-nuts/_Twwb5ULStM
// So that defer will run propoerly
func realMain() (exitStatus int) {

	// initialize ncurses
	stdscr, colors, resetScreen := initializeNcurses()

	// clean up the screen before we die
	defer func() {
		if err := recover(); err != nil {
			resetScreen()
			fmt.Fprintf(os.Stderr, "exiting from error: %s \n", err)
			ERROR.Println("exiting from error: ", err)
			exitStatus = 1
			os.Exit(1)
		}
	}()

	// draw the stuff on the screen
	msgWin, workerCountWin, durWin, reqSecWin, barsWin, scaleWin, maxWin := drawDisplay(stdscr)

	// create our various channels
	infoMsgsCh := make(chan ncursesMsg)
	exitCh := make(chan int)
	changeNumRequestersCh := make(chan interface{})
	changeNumRequestersListenerCh := make(chan interface{})
	reqMadeOnSecCh := make(chan interface{})
	reqMadeOnSecListenerCh := make(chan interface{})
	reqMadeOnSecSlaveListenerCh := make(chan interface{})
	failsOnSecCh := make(chan int)
	durationCh := make(chan int64)
	durationDisplayCh := make(chan string)
	reqSecDisplayCh := make(chan string)
	bytesPerSecCh := make(chan bytesPerSecMsg)
	bytesPerSecDisplayCh := make(chan string)
	barsToDrawCh := make(chan currentBars)

	// start all the worker goroutines
	go windowRunloop(infoMsgsCh, exitCh, changeNumRequestersCh, msgWin)
	go barsController(reqMadeOnSecListenerCh, failsOnSecCh, barsToDrawCh, reqSecDisplayCh)
	go requesterController(infoMsgsCh, changeNumRequestersListenerCh, reqMadeOnSecCh, failsOnSecCh, durationCh, bytesPerSecCh, *testUrl, *introduceRandomFails)
	go durationWinController(durationCh, durationDisplayCh)
	go bytesPerSecController(bytesPerSecCh, bytesPerSecDisplayCh)

	numRequestersBcaster := bcast.MakeNew(changeNumRequestersCh, INFO)
	numRequestersBcaster.Join(changeNumRequestersListenerCh)

	reqMadeOnSecBcaster := bcast.MakeNew(reqMadeOnSecCh, INFO)
	reqMadeOnSecBcaster.Join(reqMadeOnSecListenerCh)

	if *listen > 0 {
		port := *listen
		// we don't want to join until we start the listener
		joinUp := func() {
			reqMadeOnSecBcaster.Join(reqMadeOnSecSlaveListenerCh)
		}
		go slave.ListenForMaster(port, changeNumRequestersCh, reqMadeOnSecSlaveListenerCh, joinUp, INFO)
	} else if len(slaveList) > 0 {
		connectToSlaves(slaveList, numRequestersBcaster, reqMadeOnSecCh)
	}

	currentScale := int64(1)

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
			durWin.MovePrint(1, 1, fmt.Sprintf("%11s", msg))
			durWin.NoutRefresh()
		case msg := <-reqSecDisplayCh:
			reqSecWin.MovePrint(1, 1, fmt.Sprintf("%14s", msg))
			reqSecWin.NoutRefresh()
		case msg := <-barsToDrawCh:
			currentScale = calculateScale(msg.max)
			// 25 is the number of rows in the window, s/b dynamic or defined elsewhere
			maxWin.MovePrint(1, 1, fmt.Sprintf("%5d", msg.max))
			maxWin.NoutRefresh()
			scaleWin.MovePrint(1, 1, fmt.Sprintf("%5d", currentScale))
			scaleWin.NoutRefresh()
			updateBarsWin(msg, barsWin, *colors, currentScale)
		case msg := <-bytesPerSecDisplayCh:
			//msgAsFloat64, err := strconv.ParseFloat(msg, 64)
			//if err != nil {
			//    panic(fmt.Sprintf("converting %s failed: %v", msg, err))
			//}
			//if msgAsFloat64 > 0 {
			if msg != "      0.00" {
				INFO.Println("bytes/sec for each second: ", msg)
			}
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

func calculateScale(max int64) int64 {
	if max == 0 {
		return 1
	} else {
		// 25 is the number of rows in the window, s/b dynamic or defined elsewhere
		return int64(max/25) + 1
	}
}

func initializeNcurses() (stdscr *gc.Window, colors *colorsDefined, resetScreen resetScreenFn) {

	stdscr, err := gc.Init()
	if err != nil {
		log.Fatal(err)
	}
	defer gc.End()
	resetScreen = func() {
		gc.End()
	}

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
	scaleWin *gc.Window,
	maxWin *gc.Window,
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
	durHeight, durWidth := 4, 14
	durY := 2
	durX := ctrX + ctrWidth + 1
	stdscr.MovePrint(1, durX+1, "duration ms")
	stdscr.NoutRefresh()
	durWin = createWindow(durHeight, durWidth, durY, durX)
	durWin.Box(0, 0)
	durWin.NoutRefresh()

	// Create the requests/sec window,
	reqSecHeight, reqSecWidth := 3, 16
	reqSecY := 2
	reqSecX := durX + durWidth + 1
	stdscr.MovePrint(1, reqSecX+1, "req/s 1/5/60")
	stdscr.NoutRefresh()
	reqSecWin = createWindow(reqSecHeight, reqSecWidth, reqSecY, reqSecX)
	reqSecWin.Box(0, 0)
	reqSecWin.NoutRefresh()

	// Create the bars window, showing the moving display of bars
	secondsPerMinute := 60
	barsWidth := secondsPerMinute + 3 // we wrap after a minute
	barsHeight := 25                  // need to size this dynamically, TBD
	barsY := msgHeight + 1
	barsX := 9 // leave space for scale window
	barsWin = createWindow(barsHeight, barsWidth, barsY, barsX)
	barsWin.Box(0, 0)
	barsWin.NoutRefresh()

	// Max window, showing the max seen over the last 60 seconds
	maxWidth := 7
	maxHeight := 3
	maxY := barsY + barsHeight - 8
	maxX := 1
	stdscr.MovePrint(maxY, 1, "max:")
	stdscr.NoutRefresh()
	maxY += 1
	maxWin = createWindow(maxHeight, maxWidth, maxY, maxX)
	maxWin.Box(0, 0)
	maxWin.NoutRefresh()

	// Scale window, showing our current scaling factor for the bars display
	scaleWidth := 7
	scaleHeight := 3
	scaleY := barsY + barsHeight - 4
	scaleX := 1
	stdscr.MovePrint(scaleY, 1, "scale:")
	stdscr.NoutRefresh()
	scaleY += 1
	scaleWin = createWindow(scaleHeight, scaleWidth, scaleY, scaleX)
	scaleWin.Box(0, 0)
	scaleWin.MovePrint(1, 1, fmt.Sprintf("%5s", "1"))
	scaleWin.NoutRefresh()

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
func updateBarsWin(msg currentBars, barsWin *gc.Window, colors colorsDefined, scale int64) {

	whiteOnBlack := colors.whiteOnBlack
	redOnBlack := colors.redOnBlack
	greenOnBlack := colors.greenOnBlack
	barsWin.Erase()
	barsWin.Box(0, 0)
	edibleCopy := make([]int64, len(msg.cols))
	copy(edibleCopy, msg.cols)
	barsHeight, barsWidth := barsWin.MaxYX()
	startI := len(edibleCopy) - barsWidth
	if startI < 0 {
		startI = 0
	}
	currentSec := time.Now().Second()
	prevSec := currentSec - 1
	if prevSec < 0 {
		prevSec = 59
	}
	for row := 0; row < barsHeight-2; row++ {
		for col := range edibleCopy[startI:len(edibleCopy)] {
			if edibleCopy[col]/scale > 0 {
				turnOffColor := int16(0)
				currChar := "="
				// row is an int--32-bit, right?
				if shouldShowFail(msg.failCols[col], scale, row) {
					barsWin.ColorOff(whiteOnBlack)
					barsWin.ColorOn(redOnBlack)
					currChar = "x"
					turnOffColor = redOnBlack

				} else if col == currentSec ||
					col == currentSec-1 {
					// current second is still in progress, so make the previous second
					// green too--not precisely correct, but close enough here
					barsWin.ColorOff(whiteOnBlack)
					barsWin.ColorOn(greenOnBlack)
					turnOffColor = greenOnBlack
				}

				barsWin.MovePrint(barsHeight-2-row, col+1, currChar)

				if turnOffColor != 0 {
					barsWin.ColorOff(turnOffColor)
					barsWin.ColorOn(whiteOnBlack)
				}

				edibleCopy[col] = edibleCopy[col] - scale
			}
		}
	}
	barsWin.NoutRefresh()
}

// Called from updateBarsWin
// The scale factor would result in a fractional value if there's
// only one fail this second--we always want to show a fail marker
// if there are *any* fails, otherwise they become invisible
func shouldShowFail(numFailsThisSec int64, scale int64, rowNum int) bool {
	if numFailsThisSec/scale > int64(rowNum) ||
		rowNum == 0 && numFailsThisSec > 0 {
		return true
	} else {
		return false
	}
}

func windowRunloop(
	infoMsgsCh chan<- ncursesMsg,
	exitCh chan<- int,
	changeNumRequestersCh chan<- interface{},
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
	infoMsgsCh chan<- ncursesMsg,
	changeNumRequestersCh chan<- interface{},
	win *gc.Window,
	threadCount int,
) {
	INFO.Println("increasing threads to ", threadCount)
	infoMsgsCh <- ncursesMsg{"increasing threads", threadCount, MSG_TYPE_INFO}
	changeNumRequestersCh <- 1
}

func decreaseThreads(
	infoMsgsCh chan<- ncursesMsg,
	changeNumRequestersCh chan<- interface{},
	win *gc.Window,
	threadCount int,
) {
	INFO.Println("decreasing threads to ", threadCount)
	infoMsgsCh <- ncursesMsg{"decreasing threads", threadCount, MSG_TYPE_INFO}
	changeNumRequestersCh <- -1
}

func requesterController(
	infoMsgsCh chan<- ncursesMsg,
	changeNumRequestersListenerCh <-chan interface{},
	reqMadeOnSecCh chan<- interface{},
	failsOnSecCh chan<- int,
	durationCh chan<- int64,
	bytesPerSecCh chan<- bytesPerSecMsg,
	testUrl string,
	introduceRandomFails int,
) {

	//var chans = []chan int
	// this creates a slice associated with an underlying array
	chans := make([]chan int, 0)

	for {
		select {
		case upOrDown := <-changeNumRequestersListenerCh:
			if upOrDown == 1 {
				shutdownChan := make(chan int)
				chans = append(chans, shutdownChan)
				chanId := len(chans) - 1
				go requester(infoMsgsCh, shutdownChan, chanId, reqMadeOnSecCh, failsOnSecCh, durationCh, bytesPerSecCh, testUrl, introduceRandomFails)
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
	infoMsgsCh chan<- ncursesMsg,
	shutdownChan <-chan int,
	id int,
	reqMadeOnSecCh chan<- interface{},
	failsOnSecCh chan<- int,
	durationCh chan<- int64,
	bytesPerSecCh chan<- bytesPerSecMsg,
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

			// make the request and time it
			t0 := time.Now()
			resp, err := http.Get(thisUrl + "?" + hitId) // TBD make that appending conditional
			t1 := time.Now()
			resp.Body.Close() // this only works if ! err

			// report the duration
			duration := int64(t1.Sub(t0) / time.Millisecond)
			durationCh <- duration

			// report that we made a request this second
			nowSec := time.Now().Second()
			reqMadeOnSecCh <- nowSec

			// report on the number of bytes
			bytesPerSecCh <- bytesPerSecMsg{
				bytes:         resp.ContentLength,
				duration:      time.Duration(duration),
				receivedOnSec: nowSec,
			}
			if err == nil && resp.StatusCode == 200 {
				TRACE.Println(id, "/", i, " fetch ok ")
				// TMI! infoMsgsCh <- ncursesMsg{"request ok " + hitId, -1, MSG_TYPE_RESULT}
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
			time.Sleep(10 * time.Millisecond)
		}
		if shutdownNow {
			return
		}
	}
}

// This sends messages to both the barsToDrawCh and the durationDisplayCh--
// so should I rename this method?
func barsController(
	reqMadeOnSecListenerCh <-chan interface{},
	failsOnSecCh <-chan int,
	barsToDrawCh chan<- currentBars,
	reqSecDisplayCh chan<- string,
) {
	requestsForSecond := rb.MakeNew(INFO) // one column for each clock second
	failsForSecond := rb.MakeNew(INFO)    // one column for each clock second

	secsSeen := 0

	timeToRedraw := make(chan bool)
	go func(timeToRedraw chan bool) {
		for {
			time.Sleep(1000 * time.Millisecond)
			timeToRedraw <- true
			if secsSeen <= 60 {
				secsSeen++
			}
		}
	}(timeToRedraw)

	for {
		select {
		case msg := <-reqMadeOnSecListenerCh:
			second := msg.(int)
			requestsForSecond.IncrementAt(second)
		case second := <-failsOnSecCh:
			failsForSecond.IncrementAt(second)
		case <-timeToRedraw:
			barsToDrawCh <- currentBars{
				requestsForSecond.GetArray(),
				failsForSecond.GetArray(),
				requestsForSecond.GetMax(),
			}
			reqSecDisplayCh <- fmt.Sprintf("%d/%2.2d/%2.2d",
				requestsForSecond.GetPrevVal(),
				// won't be accurate for first five secs
				requestsForSecond.SumPrevN(5)/5,
				requestsForSecond.SumPrevN(secsSeen)/
					int64(secsSeen),
			)
		}
	}
}

func durationWinController(
	durationCh <-chan int64,
	durationDisplayCh chan<- string,
) {
	totalDurForSecond := rb.MakeNew(INFO) // total durations for each clock second
	countForSecond := rb.MakeNew(INFO)    // how many received per second
	lookbackSecs := 5
//	secsSeen := 0

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
			totalDurForSecond.ChangeHeadBy(dur)
			countForSecond.IncrementHead()
		case <-timeToRedraw:

			windowDur := totalDurForSecond.SumPrevN(lookbackSecs)
			windowCount := countForSecond.SumPrevN(lookbackSecs)

			if windowCount > 0 {
				avgDur := float64(windowDur) / float64(windowCount)
				durationDisplayCh <- fmt.Sprintf("%11.2f\n (avg last %d)", avgDur, lookbackSecs)
			} else {
				durationDisplayCh <- "0"
			}
		}
	}
}

func bytesPerSecController(bytesPerSecCh <-chan bytesPerSecMsg, bytesPerSecDisplayCh chan<- string) {

	bytesRecdForSecond := rb.MakeNew(INFO)
	durationForSecond := rb.MakeNew(INFO)
	lookbackSecs := 5

	timeToRedraw := make(chan bool)
	go func(timeToRedraw chan bool) {
		for {
			time.Sleep(1000 * time.Millisecond)
			timeToRedraw <- true
		}
	}(timeToRedraw)

	for {
		select {
		case msg := <-bytesPerSecCh:
			bytesRecdForSecond.IncrementAtBy(msg.receivedOnSec, msg.bytes)
			durationForSecond.IncrementAtBy(msg.receivedOnSec, int64(msg.duration))
		case <-timeToRedraw:
			windowDur := durationForSecond.SumPrevN(lookbackSecs)
			windowBytes := bytesRecdForSecond.SumPrevN(lookbackSecs)
			// divide-by-zero guard
			if windowDur == 0 {
				windowDur = 1
			}
			bytesPerSecDisplayCh <- fmt.Sprintf("%10.2f", float64(windowBytes)/float64(windowDur)*1000)
		}

	}
}


func connectToSlaves(
		slaveList slave.Slaves, 
		numRequestersBcaster *bcast.Bcast, 
		reqMadeOnSecCh chan<- interface{}) {

	for _, slaveAddr := range slaveList {
		INFO.Println("connecting to slave " + slaveAddr)
		conn, err := net.Dial("tcp", slaveAddr)
		if err != nil {
			panic("Dial failed:" + err.Error())
		}
		slaveChan := make(chan interface{})
		numRequestersBcaster.Join(slaveChan)
		go talkToSlave(conn, slaveChan)
		go listenToSlave(conn, reqMadeOnSecCh)
	}
}

func talkToSlave(conn net.Conn, changeNumRequestersSlaveCh <-chan interface{}) {
	for {
		select {
		case msg := <-changeNumRequestersSlaveCh:
			fmt.Fprintf(conn, "%d", msg)
		}
	}
}

func listenToSlave(c net.Conn, reqMadeOnSecCh chan<- interface{}){
	buf := make([]byte, 4096) // need to handle > 4096 in Read...
	for {
		//c.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, err := c.Read(buf)
		if err != nil || n == 0 {
			c.Close()
			break
		}
		var msg slave.MsgFromSlave
		err = json.Unmarshal(buf[:n], &msg) // reslicing using num bytes actually read
		if err != nil {
			INFO.Printf("got an error from unmarshalling slave msg: %v, the data was %s", err, buf[:n])
		}
		processMsgFromSlave(msg, reqMadeOnSecCh)
	}
}

func processMsgFromSlave(msg slave.MsgFromSlave, reqMadeOnSecCh chan<- interface{}){
	if msg.StatsForSecond != nil {
		for secStr := range msg.StatsForSecond {
			secInt, pErr := strconv.ParseInt(secStr, 10, 0)
			if pErr != nil {
				INFO.Printf("couldn't parse sec '%s' in msg from slave: %s, %v", secStr, pErr, msg)
				continue
			}
			// should make a different channel so we can pass totals? or change this channel
			// to take a "second" and a number? this is really inefficient...
			for i := int64(0); i < msg.StatsForSecond[secStr]; i++ {
				reqMadeOnSecCh<- int(secInt)
			}
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
