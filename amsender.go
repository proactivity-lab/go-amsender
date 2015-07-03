// Author  Raido Pahtma
// License MIT

package main

import "fmt"
import "os"
import "os/signal"
import "log"
import "time"
import "encoding/hex"

import "github.com/jessevdk/go-flags"
import "github.com/proactivity-lab/go-sfconnection"

const ApplicationVersionMajor = 0
const ApplicationVersionMinor = 1
const ApplicationVersionPatch = 0

var ApplicationBuildDate string
var ApplicationBuildDistro string

type HexString []byte

func (self *HexString) UnmarshalFlag(value string) error {
	data, err := hex.DecodeString(value)
	*self = data
	return err
}

func (self HexString) MarshalFlag() (string, error) {
	return hex.EncodeToString(self), nil
}

func main() {

	var opts struct {
		Positional struct {
			ConnectionString string `description:"Connectionstring sf@HOST:PORT"`
		} `positional-args:"yes"`

		Source      sfconnection.AMAddr `short:"s" long:"source" default:"0001" description:"Source of the packet (hex)"`
		Destination sfconnection.AMAddr `short:"d" long:"destination" default:"FFFF" description:"Destination of the packet (hex)"`

		Group sfconnection.AMGroup `short:"g" long:"group" default:"22" description:"Packet AM Group (hex)"`
		AmId  sfconnection.AMID    `short:"a" long:"amid" required:"true" description:"Packet AM ID (hex)"`

		Payload HexString `short:"p" long:"payload" default:"" description:"Packet payload (hex)"`

		Debug       []bool `short:"D" long:"debug" description:"Debug mode, print raw packets"`
		ShowVersion func() `short:"V" long:"version" description:"Show application version"`
	}

	opts.ShowVersion = func() {
		if ApplicationBuildDate == "" {
			ApplicationBuildDate = "YYYY-mm-dd_HH:MM:SS"
		}
		if ApplicationBuildDistro == "" {
			ApplicationBuildDistro = "unknown"
		}
		fmt.Printf("amsender %d.%d.%d (%s %s)\n", ApplicationVersionMajor, ApplicationVersionMinor, ApplicationVersionPatch, ApplicationBuildDate, ApplicationBuildDistro)
		os.Exit(0)
	}

	_, err := flags.Parse(&opts)
	if err != nil {
		fmt.Printf("Argument parser error: %s\n", err)
		os.Exit(1)
	}

	host, port, err := sfconnection.ParseSfConnectionString(opts.Positional.ConnectionString)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	dsp := sfconnection.NewMessageDispatcher(new(sfconnection.Message))
	sfc := sfconnection.NewSfConnection()
	sfc.AddDispatcher(dsp)

	// Configure logging
	logformat := log.Ldate | log.Ltime | log.Lmicroseconds
	var logger *log.Logger
	if len(opts.Debug) > 0 {
		if len(opts.Debug) > 1 {
			logformat = logformat | log.Lshortfile
		}
		logger = log.New(os.Stdout, "INFO:  ", logformat)
		sfc.SetDebugLogger(log.New(os.Stdout, "DEBUG: ", logformat))
		sfc.SetInfoLogger(logger)
	} else {
		logger = log.New(os.Stdout, "", logformat)
	}
	sfc.SetWarningLogger(log.New(os.Stdout, "WARN:  ", logformat))
	sfc.SetErrorLogger(log.New(os.Stdout, "ERROR: ", logformat))

	// Connect to the host
	err = sfc.Connect(host, port)
	if err != nil {
		logger.Printf("unable to connect to %s:%d\n", host, port)
		os.Exit(1)
	}
	logger.Printf("connected to %s:%d\n", host, port)

	// Create the message
	msg := dsp.NewPacket().(*sfconnection.Message)
	msg.SetDestination(opts.Destination)
	msg.SetSource(opts.Source)
	msg.SetType(opts.AmId)
	msg.SetGroup(opts.Group)
	msg.Payload = []byte(opts.Payload)

	// Send the message
	err = sfc.Send(msg)
	if err == nil {
		logger.Printf("sent %s\n", msg)
	} else {
		logger.Printf("send error %s\n", err)
	}

	// Set up signals to close nicely on Control+C
	signals := make(chan os.Signal)
	signal.Notify(signals, os.Interrupt, os.Kill)

	// Wait a bit for the message to get sent and then shut down
	for interrupted := false; interrupted == false; {
		select {
		case <-time.After(2 * time.Second):
			if sfc.Connected() == false {
				signal.Stop(signals)
				interrupted = true
			} else {
				go sfc.Disconnect()
			}
		case sig := <-signals:
			signal.Stop(signals)
			logger.Printf("signal %s\n", sig)
			sfc.Disconnect()
			interrupted = true
		}
	}
}
