package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/urfave/cli"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"strings"
	"time"
)

var version string
var log *zap.SugaredLogger
var debug = true
var exit = make(chan bool, 1)

func init() {
	consoleEncoderConfig := zap.NewDevelopmentEncoderConfig()
	// naive systemd detection. Drop timestamp if running under it
	// if os.Getenv("INVOCATION_ID") != "" || os.Getenv("JOURNAL_STREAM") != "" {
	// 	consoleEncoderConfig.TimeKey = ""
	// }
	consoleEncoderConfig.TimeKey=""
	consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderConfig)
	consoleStderr := zapcore.Lock(os.Stderr)
	_ = consoleStderr

	// if needed point differnt priority log to different place
	highPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.ErrorLevel
	})
	lowPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl < zapcore.ErrorLevel
	})
	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, os.Stderr, lowPriority),
		zapcore.NewCore(consoleEncoder, os.Stderr, highPriority),
	)
	logger := zap.New(core)
	if debug {
		logger = logger.WithOptions(
			zap.Development(),
			zap.AddCaller(),
			zap.AddStacktrace(highPriority),
		)
	} else {
		logger = logger.WithOptions(
			zap.AddCaller(),
		)
	}
	log = logger.Sugar()

}

func main() {
	app := cli.NewApp()
	app.Name = "rodrev"
	app.Description = "rodrev client"
	app.Version = version
	app.HideHelp = true
	app.Flags = []cli.Flag{
		cli.BoolFlag{Name: "help, h", Usage: "show help"},
		cli.StringFlag{
			Name:   "mqtt-url",
			Value:  "tcp://mqtt:mqtt@127.0.0.1:1883",
			Usage:  "URL for the MQ server. Use tls:// to enable encryption",
			EnvVar: "RF_MQTT_URL",
		},
	}
	app.Action = func(c *cli.Context) error {
		if c.Bool("help") {
			cli.ShowAppHelp(c)
			os.Exit(1)
		}

		tr := zerosvc.NewTransport(
			zerosvc.TransportMQTT,
			c.String("mqtt-url"),
			zerosvc.TransportMQTTConfig{},
		)
		host,_ := os.Hostname()
		rn := make([]byte,4)
		rand.Read(rn)
		nodename := "rf-client" + host + hex.EncodeToString(rn)
		node := zerosvc.NewNode(nodename)
		err := tr.Connect()
		if err != nil {
			log.Panicf("can't connect: %s",err)
		}
		node.SetTransport(tr)
		ch, err := node.GetEventsCh("rf/heartbeat/#")
		if err != nil {
			log.Panicf("can't connect: %s",err)
		}
		services := make(map[string]map[string]bool,0)
		totalDiscoveryTime := time.After(time.Second * 30)
		exit := false
		log.Info("running service discovery")
		for {
			if exit {break}
			select {
			case ev := <-ch:
				path := strings.Split(ev.RoutingKey, "/")
				if len(path) < 2 {
					log.Errorf("path too short: %s", ev.RoutingKey)
				}
				fqdn := path[len(path)-1]

				var hb zerosvc.Heartbeat
				err := json.Unmarshal(ev.Body, &hb)
				if err != nil {
					log.Errorf("error unmarshalling %s: %s", string(ev.Body), err)
					continue
				}
				for k, _ := range hb.Services {
					if _, ok := services[k]; !ok {
						services[k] = make(map[string]bool)
						services[k][fqdn] = true
					}
				}
				log.Infof("HB: %+v|%s", hb)
			case <-time.After(4 * time.Second):
					exit = true
			case <- totalDiscoveryTime:
				exit = true
			}
		}
		for svc, hosts := range services {
			log.Infof("%s:",svc)
			for host, _ := range hosts {
				log.Infof("    %s",host)
			}
		}

		return nil
	}
	app.Commands = []cli.Command{
		{
			Name:    "rem",
			Aliases: []string{"a"},
			Usage:   "example cmd",
			Action: func(c *cli.Context) error {
				log.Warn("running example cmd")
				return nil
			},
		},
		{
			Name:    "add",
			Aliases: []string{"a"},
			Usage:   "example cmd",
			Action: func(c *cli.Context) error {
				log.Warn("running example cmd")
				return nil
			},
		},
	}
	// to sort do that
	//sort.Sort(cli.FlagsByName(app.Flags))
	//sort.Sort(cli.CommandsByName(app.Commands))
	app.Run(os.Args)
}
