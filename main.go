package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jacobsa/go-serial/serial"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type maraXCollector struct {
	info            *prometheus.Desc
	steamTemp       *prometheus.Desc
	steamTargetTemp *prometheus.Desc
	hxTemp          *prometheus.Desc
	readyCountdown  *prometheus.Desc
	heating         *prometheus.Desc

	serialPort io.ReadWriteCloser
}

// maraXStatus is all the data returned by the Mara X serial UART port.
type maraXStatus struct {
	// version is the firmware version of the thing.
	version string
	// mode is the mode the machine is in. C for coffee priority, V for vapour (steam) priority.
	mode mode
	// steamTemp is the current steam temperature
	steamTemp uint16
	// steamTargetTemp is the steam target temperature it wants to reach
	steamTargetTemp uint16
	// hxTemp is the current temperature of the heat exchanger
	hxTemp uint16
	// readyCountdown shows if the machine is in "fast heating" mode. If so, it
	// will start somewhere at 1500 and eventually end up at 0 once it's done.
	readyCountdown uint16
	// heating indicates whether the heating element is on or off.
	heating bool
}

type mode string

const (
	coffee mode = "coffee"
	steam  mode = "steam"

	coffeeMode = "C"
	steamMode  = "V"

	errReadTimeout = "timeout reading from serial device"
)

var (
	serialDevice = flag.String("serial-dev", "/dev/serial0", "path to the serial device to read")
	port         = flag.Int("port", 8080, "port for the http server to listen on")
)

func newMaraXCollector() (*maraXCollector, error) {
	options := serial.OpenOptions{
		PortName:        *serialDevice,
		BaudRate:        9600,
		DataBits:        8,
		StopBits:        1,
		MinimumReadSize: 4,
	}

	port, err := serial.Open(options)
	if err != nil {
		return nil, fmt.Errorf("unable to open serial device at %s: %w", *serialDevice, err)
	}

	return &maraXCollector{
		serialPort: port,
		info: prometheus.NewDesc(
			"mara_x_info",
			"Contains information about the Mara X machine.",
			[]string{"version", "mode"}, nil,
		),
		steamTemp: prometheus.NewDesc(
			"mara_x_steam_temperature",
			"The steam target temperature it wants to reach.",
			nil, nil,
		),
		steamTargetTemp: prometheus.NewDesc(
			"mara_x_steam_target_temperature",
			"The current steam temperature.",
			nil, nil,
		),
		hxTemp: prometheus.NewDesc(
			"mara_x_hx_temperature",
			"Temperature of the heat exchanger.",
			nil, nil,
		),
		readyCountdown: prometheus.NewDesc(
			"mara_x_ready_countdown",
			"Shows if the machine is in 'fast heating' mode.",
			nil, nil,
		),
		heating: prometheus.NewDesc(
			"mara_x_heating",
			"Indicates whether the heating element is on or off.",
			nil, nil,
		),
	}, nil
}

func (collector *maraXCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.steamTemp
}

func (collector *maraXCollector) Collect(ch chan<- prometheus.Metric) {
	status, err := collector.collectDataFromSerial()
	if err != nil {
		log.Printf("error collecting metrics from serial port: %s", err)
		return
	}

	ch <- prometheus.MustNewConstMetric(
		collector.info, prometheus.GaugeValue, float64(1), status.version, string(status.mode),
	)
	ch <- prometheus.MustNewConstMetric(collector.steamTemp, prometheus.GaugeValue, float64(status.steamTemp))
	ch <- prometheus.MustNewConstMetric(collector.steamTargetTemp, prometheus.GaugeValue, float64(status.steamTargetTemp))
	ch <- prometheus.MustNewConstMetric(collector.hxTemp, prometheus.GaugeValue, float64(status.hxTemp))
	ch <- prometheus.MustNewConstMetric(collector.readyCountdown, prometheus.GaugeValue, float64(status.readyCountdown))

	heating := 0
	if status.heating {
		heating = 1
	}
	ch <- prometheus.MustNewConstMetric(collector.heating, prometheus.GaugeValue, float64(heating))
}

func main() {
	flag.Parse()
	collector, err := newMaraXCollector()
	if err != nil {
		log.Fatal(err)
	}
	prometheus.MustRegister(collector)
	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(fmt.Sprintf(":%v", *port), nil)
}

func (collector *maraXCollector) collectDataFromSerial() (*maraXStatus, error) {
	line, err := collector.readSerialLine()
	if err != nil {
		return nil, err
	}
	return parseLine(line)
}

func (collector *maraXCollector) readSerialLine() ([]byte, error) {
	// TODO: add "warm up" phase when getting bad data in the beginning.
	data, err := readLine(collector.serialPort, time.Second*1)
	if err != nil {
		return nil, fmt.Errorf("unable to read line: %w", err)
	}
	return data, nil
}

func parseLine(l []byte) (*maraXStatus, error) {
	line := string(l)
	line = strings.TrimSuffix(line, "\r\n")

	parts := strings.Split(string(line), ",")
	if len(parts) != 6 {
		return nil, fmt.Errorf(
			"unable to parse line %s, it does not contain expected parts", line,
		)
	}

	modeVersion := strings.Split(parts[0], "")
	if len(modeVersion) < 2 {
		return nil, fmt.Errorf(
			"unable to parse line %s, the mode and version parts could not be found", line,
		)
	}

	steamTemp, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, err
	}

	steamTargetTemp, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, err
	}

	hxTemp, err := strconv.Atoi(parts[3])
	if err != nil {
		return nil, err
	}

	readyCountdown, err := strconv.Atoi(parts[4])
	if err != nil {
		return nil, err
	}

	heating, err := strconv.ParseBool(parts[5])

	mode := coffee
	if modeVersion[0] == steamMode {
		mode = steam
	}

	return &maraXStatus{
		mode:            mode,
		version:         strings.Join(modeVersion[1:], ""),
		steamTemp:       uint16(steamTemp),
		steamTargetTemp: uint16(steamTargetTemp),
		hxTemp:          uint16(hxTemp),
		readyCountdown:  uint16(readyCountdown),
		heating:         heating,
	}, err
}

func readLine(rwc io.ReadWriteCloser, timeout time.Duration) ([]byte, error) {
	b := make(chan []byte)
	e := make(chan error)

	go func() {
		reader := bufio.NewReader(rwc)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			e <- err
		} else {
			b <- line
		}
		close(b)
		close(e)
	}()

	select {
	case line := <-b:
		return line, nil
	case err := <-e:
		return nil, err
	case <-time.After(timeout):
		return nil, errors.New(errReadTimeout)
	}
}
