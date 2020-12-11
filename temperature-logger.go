package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	client "github.com/influxdata/influxdb1-client/v2"
)

func bp2string(bp client.BatchPoints) (bytes.Buffer, error) {
	// https://github.com/influxdata/influxdb1-client/blob/master/v2/client.go#L367
	var b bytes.Buffer

	for _, p := range bp.Points() {
		if p == nil {
			continue
		}
		if _, err := b.WriteString(p.PrecisionString(bp.Precision())); err != nil {
			return b, err
		}

		if err := b.WriteByte('\n'); err != nil {
			return b, err
		}
	}

	return b, nil
}

func write2spool(bp client.BatchPoints, spool string) {
	// each hour a new file
	t := time.Now()
	filename := spool + "/" + t.Format("2006010215") + ".influx"

	// append datapoints
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening spoolfile ", filename, ": ", err.Error())
		return
	}
	defer f.Close()

	buf, err := bp2string(bp)
	if err != nil {
		fmt.Println("Error converting datapoints to buffer: ", err.Error())
		return
	}

	if _, err := f.Write(buf.Bytes()); err != nil {
		fmt.Println("Error writing to spoolfile ", filename, ": ", err.Error())
		return
	}
}

func main() {
	// process commandline args
	var host string
	var port string
	var database string
	var datapoint string
	var spool string
	var sleep int
	flag.StringVar(&host, "host", "127.0.0.1", "InfluxDB host")
	flag.StringVar(&port, "port", "8086", "InfluxDB port")
	flag.StringVar(&database, "database", "temperature", "InfluxDB database")
	flag.StringVar(&datapoint, "datapoint", "temp", "InfluxDB datapoint name")
	flag.StringVar(&spool, "spool", "", "spool directory")
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

		err = influx.Write(bp)
		if err != nil {
			if spool != "" {
				write2spool(bp, spool)
			}
		}

		time.Sleep(time.Duration(sleep) * time.Second)
	}
}
