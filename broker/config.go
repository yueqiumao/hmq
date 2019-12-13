/* Copyright (c) 2018, joy.zhou <chowyu08@gmail.com>
 */
package broker

// "github.com/fhmq/hmq/logger"

type Config struct {
	Worker   int     `json:"workerNum"`
	HTTPPort string  `json:"httpPort"`
	Host     string  `json:"host"`
	Port     string  `json:"port"`
	WsPath   string  `json:"wsPath"`
	WsPort   string  `json:"wsPort"`
	Debug    bool    `json:"debug"`
	Plugin   Plugins `json:"plugins"`
}

type Plugins struct {
	Auth   string
	Bridge string
}

type RouteInfo struct {
	Host string `json:"host"`
	Port string `json:"port"`
}

type TLSInfo struct {
	Verify   bool   `json:"verify"`
	CaFile   string `json:"caFile"`
	CertFile string `json:"certFile"`
	KeyFile  string `json:"keyFile"`
}

var DefaultConfig *Config = &Config{
	Worker: 4096,
	Host:   "0.0.0.0",
	Port:   "1883",
}
