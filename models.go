package main

type MotorControl struct {
	Id         int   `json:"id"`
	Channel    uint8 `json:"channel"`
	IsReversed bool  `json:"reversed"`
	Speed      uint8 `json:"speed"`
}

type Sensor struct {
	Id      int   `json:"id"`
	Address uint8 `json:"address"`
}

type TurnOut struct {
	Id         int   `json:"id"`
	ForwardPin uint8 `json:"forwardPin"`
	TurnOutPin uint8 `json:"turnOutPin"`
	TurnedOut  bool  `json:"turnedOut"`
}
