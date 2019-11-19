package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	client "github.com/influxdata/influxdb1-client/v2"
)

func main() {
	// process commandline args
	var host string
	var port string
	var database string
	var datapoint string
	var sleep int
	flag.StringVar(&host, "host", "127.0.0.1", "InfluxDB host")
	flag.StringVar(&port, "port", "8086", "InfluxDB port")
	flag.StringVar(&database, "database", "temperature", "InfluxDB database")
	flag.StringVar(&datapoint, "datapoint", "temp", "InfluxDB datapoint name")
	flag.IntVar(&sleep, "sleep", 20, "sleep between measurements (in seconds)")
	flag.Parse()

	// setting up InfluxDB client
	influx, err := client.NewHTTPClient(client.HTTPConfig{
		Addr: "http://" + host + ":" + port,
	})
	if err != nil {
		fmt.Println("Error creating InfluxDB client: ", err.Error())
		panic("exit")
	}
	defer influx.Close()

	nameRe := regexp.MustCompile(`\d\d-[0-9a-f]+`)
	tempRe := regexp.MustCompile(` t=(\d+)`)

	for {
		// create datapoints batch
		bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
			Database:  database,
			Precision: "s",
		})

		// get all one-wire sensors
		sensors, err := filepath.Glob("/sys/bus/w1/devices/*-*/w1_slave")
		if err != nil {
			fmt.Println("Error reading sensor directories: ", err.Error())
			panic("exit")
		}

		for _, sensor := range sensors {
			// extract name
			name := nameRe.FindString(sensor)
			if name == "" {
				fmt.Println("Could not extract sensor name: ", sensor)
				break
			}

			// read temperature from sensor
			content, err := ioutil.ReadFile(sensor)
			if err != nil {
				fmt.Println("Error reading sensor file: ", err.Error())
				break
			}

			// extract temperature
			matches := tempRe.FindSubmatch(content)
			if matches == nil {
				fmt.Println("Could not extract temperature: ", content)
				break
			}

			// convert temperature to float
			temp, err := strconv.ParseFloat(string(matches[1]), 64)
			if err != nil {
				fmt.Println("Error converting temperature: ", err.Error())
				break
			}
			temp = temp / 1000

			// add datapoint to batch
			tags := map[string]string{"sensor": name}
			fields := map[string]interface{}{"value": temp}
			pt, err := client.NewPoint(datapoint, tags, fields, time.Now())
			if err != nil {
				fmt.Println("Error: ", err.Error())
				break
			}
			bp.AddPoint(pt)
		}

		influx.Write(bp)

		time.Sleep(time.Duration(sleep) * time.Second)
	}
}
