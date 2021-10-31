package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/tarm/serial"
)

type logWriter struct {
}

func (writer logWriter) Write(bytes []byte) (int, error) {
	return fmt.Print(time.Now().UTC().Format("2006-01-02T15:04:05.999Z") + " " + string(bytes))
}

func main() {
	log.SetFlags(0)
	log.SetOutput(new(logWriter))

	config := &serial.Config{
		Name:        "/dev/ttyACM0",
		Baud:        9600,
		ReadTimeout: 1,
		Size:        8,
	}

	log.Println("Opening connection to serial port")
	con, err := OpenConnection(config)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Opening connection to STOMP server")
	stompConnection, err := NewStompConnection("http://localhost:8080/pi-train-broker/agent-websocket")
	if err != nil {
		log.Fatal(err)
	}

	receiptId, _ := uuid.NewUUID()
	stompConnection.Connect(receiptId)
	log.Println("Listening for response on websocket")
	go ctrlCHandler()
	go stompConnection.Listen()
	go listenForMotorCommands(con, stompConnection)
	go listenForSensorCommands(con, stompConnection)
	go listenForTurnOutCommands(con, stompConnection)
	var callback = func(line []byte) {
		err = stompConnection.Send("/topic/response", line, "application/json", "")
		if err != nil {
			log.Println("Error writing serial output to response topic")
			log.Panic(err)
		}
	}

	log.Println("Listening for data from serial port")
	err = con.Listen(callback)
	if err != nil {
		log.Fatal(err)
	}
}

func ctrlCHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("Ctrl+C pressed, exiting")
		os.Exit(0)
	}()
}

func listenForMotorCommands(
	serialConnection *SerialConnection,
	stompConnection *StompConnection,
) {
	log.Println("Subscribing to: /topic/motor-control")
	c, _, err := stompConnection.AddSubscription("/topic/motor-control")
	if err != nil {
		log.Fatal(err)
		return
	}

	var mc MotorControl
	var msg StompMessage
	var ok = true
	for ok {
		msg, ok = <-c
		if ok {
			err := json.Unmarshal([]byte(msg.Body), &mc)
			if err != nil {
				log.Println("Could not process message body from: /topic/motor-control")
				log.Panic(err)
				continue
			}

			if mc.IsReversed {
				log.Println(fmt.Sprintf("Setting channel: %d to reverse at to speed: %d ", mc.Channel, mc.Speed))
			} else {
				log.Println(fmt.Sprintf("Setting channel: %d to move forward at to speed: %d ", mc.Channel, mc.Speed))
			}

			_, err = serialConnection.WriteMotorCommand(mc.Channel, mc.Speed, mc.IsReversed)
			if err != nil {
				log.Println("Error writing motor control command to serial port")
				log.Panic(err)
			}
		}
	}
}

func listenForSensorCommands(
	serialConnection *SerialConnection,
	stompConnection *StompConnection,
) {
	log.Println("Subscribing to: /topic/sensor")
	c, _, err := stompConnection.AddSubscription("/topic/sensor")
	if err != nil {
		log.Fatal(err)
		return
	}

	var msg StompMessage
	var ok = true
	var sensor Sensor
	for ok {
		msg, ok = <-c
		if ok {
			err := json.Unmarshal([]byte(msg.Body), &sensor)
			if err != nil {
				log.Println("Could not process message body from: /topic/sensor")
				log.Panic(err)
				continue
			}

			log.Println(fmt.Sprintf("Reading from sensor %x", sensor.Address))
			_, err = serialConnection.WriteSensorReadCommand(sensor.Address, 2)
			if err != nil {
				log.Println("Error writing sesnor read command to serial port")
				log.Panic(err)
			}
		}
	}
}

func listenForTurnOutCommands(
	serialConnection *SerialConnection,
	stompConnection *StompConnection,
) {
	log.Println("Subscribing to: /topic/turn-out")
	c, _, err := stompConnection.AddSubscription("/topic/turn-out")
	if err != nil {
		log.Fatal(err)
		return
	}

	var msg StompMessage
	var ok = true
	var turnOut TurnOut
	for ok {
		msg, ok = <-c
		if ok {
			err := json.Unmarshal([]byte(msg.Body), &turnOut)
			if err != nil {
				log.Println("Could not process message body from: /topic/turn-out")
				log.Panic(err)
				continue
			}

			if turnOut.TurnedOut {
				log.Println(fmt.Sprintf("Sending a pulse to pin %d", turnOut.TurnOutPin))
				_, err = serialConnection.WriteTurnOutCommand(turnOut.TurnOutPin)
			} else {
				log.Println(fmt.Sprintf("Sending a pulse to pin %d", turnOut.ForwardPin))
				_, err = serialConnection.WriteTurnOutCommand(turnOut.ForwardPin)
			}

			if err != nil {
				log.Println("Error writing turn out command to serial port")
				log.Panic(err)
			}
		}
	}
}
