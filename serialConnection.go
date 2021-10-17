package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"

	"github.com/tarm/serial"
)

type State struct {
	Cmd uint8 `json:"cmd"`
}

type SensorState struct {
	Cmd     uint8   `json:"cmd"`
	Address uint8   `json:"address"`
	States  []uint8 `json:"states"`
}

type SerialConnection struct {
	SerialPort *serial.Port
	channels   map[byte][]chan SensorState
}

func OpenConnection(c *serial.Config) (s *SerialConnection, err error) {
	serialPort, err := serial.OpenPort(c)
	if err != nil {
		return nil, err
	}

	return &(SerialConnection{
		SerialPort: serialPort,
		channels:   map[byte][]chan SensorState{},
	}), nil
}

func (sc *SerialConnection) AddListenerForSensor(
	address uint8,
) (c chan SensorState) {
	var channel = make(chan SensorState)
	var list, ok = sc.channels[address]
	if ok {
		sc.channels[address] = append(list, channel)
	} else {
		list = make([]chan SensorState, 1)
		list[0] = channel
		sc.channels[address] = list
	}

	return channel
}

func (sc *SerialConnection) Listen(callback func(line []byte)) (err error) {
	for {
		var scanner = bufio.NewScanner(sc.SerialPort)
		for scanner.Scan() {
			line := scanner.Bytes()
			log.Println(string(line))
			var state State
			var err = json.Unmarshal(line, &state)
			if err == nil {
				log.Println(fmt.Sprintf("Respose for cmd = %d", state.Cmd))
				callback(line)
				if state.Cmd == 4 {
					var sensorState SensorState
					err = json.Unmarshal(line, &sensorState)
					if err == nil {
						var channels, ok = sc.channels[sensorState.Address]
						if ok {
							for _, value := range channels {
								value <- sensorState
							}
						}
					}
				}
			}
		}

		var err = scanner.Err()
		if err != nil {
			return err
		}
	}
}

func (sc *SerialConnection) WriteMotorCommand(
	channel uint8,
	speed uint8,
	isReversed bool,
) (n int, err error) {
	var value0 byte = byte(speed / 0x40)
	if isReversed {
		value0 += 0x04
	}

	var value1 byte = byte(speed % 0x40)
	var output = make([]byte, 5)
	output[0] = 0x01
	output[1] = channel
	output[2] = value0
	output[3] = value1
	output[4] = 0xFF
	return sc.SerialPort.Write(output)
}

func (sc *SerialConnection) WriteSensorReadCommand(
	address uint8,
	length uint8,
) (n int, err error) {
	var output = make([]byte, 3)
	output[0] = 0x04
	output[1] = address
	output[2] = 0xFF
	return sc.SerialPort.Write(output)
}

func (sc *SerialConnection) WriteTurnOutCommand(pin uint8) (n int, err error) {
	var output = make([]byte, 3)
	output[0] = 0x02
	output[1] = pin
	output[2] = 0xFF
	return sc.SerialPort.Write(output)
}
