/* Copyright (c) 2018, joy.zhou <chowyu08@gmail.com>
 */
package broker

type Config struct {
	Host   string `json:"host"`
	Port   string `json:"port"`
	WsPath string `json:"wsPath"`
	WsPort string `json:"wsPort"`
}

var DefaultConfig *Config = &Config{
	Host: "0.0.0.0",
	Port: "1883",
}
