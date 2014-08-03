package slave

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"
)


// is this really the way to share global loggers?
var (
	INFO *log.Logger
)

// this is what we send the master
type MsgFromSlave struct {
	StatsForSecond map[string] int64
	Status string //generic, probably just for testing
}

// a slice of strings holding ip:port combos
type Slaves []string

// Now, for our new type, implement the two methods of
// the flag.Value interface...
// String is the method to format the flag's value, part of the flag.Value interface.
// The String method's output will be used in diagnostics.
func (z *Slaves) String() string {
	return fmt.Sprint(*z)
}

// The second method is Set(value string) error
func (z *Slaves) Set(value string) error {
	var validAddr = regexp.MustCompile(z.validIpRegex())

	for _, ipport := range strings.Split(value, ",") {
		if !validAddr.Match([]byte(ipport)) {
			return errors.New("Your '" + ipport + "' doesn't look like an ip:port")
		}
		*z = append(*z, ipport)
	}
	return nil
}

func (i *Slaves) validIpRegex() string {

	// http://stackoverflow.com/questions/53497/regular-expression-that-matches-valid-ipv6-addresses
	IPV4SEG := "(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])"
	IPV4ADDR := "(" + IPV4SEG + "\\.){3,3}" + IPV4SEG
	IPV6SEG := "[0-9a-fA-F]{1,4}"
	fulladdr := "(" + IPV6SEG + ":){7,7}" + IPV6SEG               // 1:2:3:4:5:6:7:8
	collapse7 := "(" + IPV6SEG + ":){1,7}:"                       // 1::                                 1:2:3:4:5:6:7::
	collapse6 := "(" + IPV6SEG + ":){1,6}:" + IPV6SEG             // 1::8               1:2:3:4:5:6::8   1:2:3:4:5:6::8
	collapse5 := "(" + IPV6SEG + ":){1,5}(:" + IPV6SEG + "){1,2}" // 1::7:8             1:2:3:4:5::7:8   1:2:3:4:5::8
	collapse4 := "(" + IPV6SEG + ":){1,4}(:" + IPV6SEG + "){1,3}" // 1::6:7:8           1:2:3:4::6:7:8   1:2:3:4::8
	collapse3 := "(" + IPV6SEG + ":){1,3}(:" + IPV6SEG + "){1,4}" // 1::5:6:7:8         1:2:3::5:6:7:8   1:2:3::8
	collapse2 := "(" + IPV6SEG + ":){1,2}(:" + IPV6SEG + "){1,5}" // 1::4:5:6:7:8       1:2::4:5:6:7:8   1:2::8
	collapse1 := IPV6SEG + ":((:" + IPV6SEG + "){1,6})"           // 1::3:4:5:6:7:8     1::3:4:5:6:7:8   1::8
	collapse0 := ":((:" + IPV6SEG + "){1,7}|:)"                   // ::2:3:4:5:6:7:8    ::2:3:4:5:6:7:8  ::8       ::
	linklocal := "fe80:(:" + IPV6SEG + "){0,4}%[0-9a-zA-Z]{1,}"   // fe80::7:8%eth0     fe80::7:8%1  (link-local IPv6 addresses with zone index)
	ip4mapped := "::(ffff(:0{1,4}){0,1}:){0,1}" + IPV4ADDR        // ::255.255.255.255  ::ffff:255.255.255.255  ::ffff:0:255.255.255.255 (IPv4-mapped IPv6 addresses and IPv4-translated addresses)
	ip4embedd := "(" + IPV6SEG + ":){1,4}:" + IPV4ADDR            // 2001:db8:3:4::192.0.2.33  64:ff9b::192.0.2.33 (IPv4-Embedded IPv6 Address)
	IPV6ADDR := "(" + fulladdr + "|" + collapse7 + "|" + collapse6 + "|" +
		collapse5 + "|" + collapse4 + "|" + collapse3 + "|" + collapse2 + "|" +
		collapse1 + "|" + collapse0 + "|" + linklocal + "|" + ip4mapped + "|" + ip4embedd + ")"
	IPADDR := "(" + IPV4ADDR + "|" + IPV6ADDR + ")"

	IPPORT := "^" + IPADDR + ":\\d+$"
	return IPPORT
}

func ListenForMaster(
			port int, 
			changeNumRequestersCh chan interface{}, 
			reqMadeOnSecSlaveListenerCh chan interface{},
			joinBcastCb func(),
			infoLog *log.Logger, //better way?
		) {
	INFO = infoLog

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatal(err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		handleConnectionFromMaster(conn, changeNumRequestersCh, reqMadeOnSecSlaveListenerCh, joinBcastCb)
	}
}


func handleConnectionFromMaster(
			c net.Conn, 
			changeNumRequestersCh chan<- interface{},
			reqMadeOnSecSlaveListenerCh <-chan interface{},
			joinBcastCb func(),
		) {
	buf := make([]byte, 4096)

	for {
		//c.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, err := c.Read(buf)
INFO.Println("we read something from master! ", n, err)
		if err != nil || n == 0 {
			c.Close()
			break
		}

		//n, err = c.Write(buf[0:n])
		delta := string(buf[:n])
		requesterDelta, err := strconv.ParseInt(delta, 10, 0)
		if err != nil {
			INFO.Println("got a wonky message from the network: %s (%s)", delta, requesterDelta)
			break
		}
		go handleStatsToMaster(c, reqMadeOnSecSlaveListenerCh, joinBcastCb)
		changeNumRequestersCh <- int(requesterDelta)
	}
	log.Printf("Connection from %v closed.", c.RemoteAddr())
}

func handleStatsToMaster (
			c net.Conn, 
			reqMadeOnSecSlaveListenerCh <-chan interface{},
			joinBcastCb func(),
		){

	//now that we're ready to start listening, we can join the bcaster
INFO.Println("about to join, our channel is ", reqMadeOnSecSlaveListenerCh)
	joinBcastCb()

	for {
		select {
		case msg :=  <-reqMadeOnSecSlaveListenerCh:
	INFO.Println("the slave got a reqMadeOnSecs msg from itself")
			second := msg.(int)
	INFO.Println("about to sendStatsToMaster ", second)
			sendStatsToMaster(c, second)
	INFO.Println("done sending stats to master")
		}
	}
}

func makeNewMsg() (MsgFromSlave){
	msg := new(MsgFromSlave)
	msg.StatsForSecond = make(map[string] int64)
	return *msg
}

// we actually want to collect these, and only send them once a second
// this is a first draft...
func sendStatsToMaster(c net.Conn, second int){

	msg := makeNewMsg()
	//msg.Status = "ok" //not actually doing anything with this on the master yet
	secondStr := fmt.Sprintf("%d", second);
	msg.StatsForSecond[secondStr] = 1 // would accumulate them
	json, err := json.Marshal(msg)
	if err != nil {
		panic(fmt.Sprintf("can't marshal that %v", err))
	}

	_, writeErr := c.Write(json)
	if writeErr != nil {
		c.Close()
		// actually, this might mean that the server has shut down, don't need
		INFO.Println("writing json to connection failed: %v", writeErr)
		panic(fmt.Sprintf("writing json to connection failed: %v", writeErr))
	}
}
