package main

import (
	"github.com/tarm/serial"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//var reg regexp·Regexp

//func init() {
//	reg, _ := regexp.Compile("[^0-9]+")
//}

func initialize_arduino() {
	if conf.Serial.Tty == "none" {
		return
	}
	lock.Lock()
	// initialize maps
	n := len(conf.Sensors.Arduino_linear)
	arduino_linear_stat = make(map[string]int, n)
	arduino_prev_linear_stat = make(map[string][]int, n)
	arduino_cache_stat = make(map[string]int, n)
	n = len(conf.Sensors.Arduino_exp)
	arduino_exp_stat = make(map[string]int, n)
	arduino_prev_exp_stat = make(map[string][]int, n)
	history_setup()
	arduino_connected = false
	arduino_comm_time = 0
	arduino_total_fail_read = 0
	lock.Unlock()
}

func read_arduino() {
	if conf.Serial.Tty == "none" {
		return
	}
	if conf.Verbose {
		log.Println("Arduino stats")
	}
	reply := ""
	for _, s := range conf.Sensors.Arduino_linear {
		log.Printf("sent instruction for: %s", s)
		validated := 0
		msg := s + "?"
		reply = comm2_arduino(msg)
		if reply != "null" {
			output, err := strconv.Atoi(reply)
			if err != nil {
				log.Printf("Failed conversion: %s\n", err)
				validated = last_linear(s)
				arduino_total_fail_read = arduino_total_fail_read + 1
			} else {
				validated = output
			}
			add_linear(s, output)
		} else {
			log.Printf("failed read, using cached value\n")
			validated = last_linear(s)
			arduino_cache_stat[s] = arduino_cache_stat[s] + 1
			arduino_total_fail_read = arduino_total_fail_read + 1
		}
		reply = ""
		if arduino_cache_stat[s] > conf.Analysis.Cache_age {
			validated = 0
			arduino_cache_stat[s] = 0
		}
		lock.Lock()
		arduino_linear_stat[s] = validated
		lock.Unlock()
		time.Sleep(time.Second * 2)
	}
	time.Sleep(time.Second * 2)
	for _, s := range conf.Sensors.Arduino_exp {
		log.Printf("sent instruction for: %s", s)
		validated := 0
		msg := s + "?"
		reply = comm2_arduino(msg)
		if reply != "null" {
			output, err := strconv.Atoi(reply)
			if err != nil {
				log.Printf("failed conversion: %s\n", err)
				validated = last_exp(s)
				arduino_total_fail_read = arduino_total_fail_read + 1
			} else {
				validated = output
			}
			// add every value we recieve to the history
			add_exp(s, validated)
		} else {
			log.Printf("failed read, using cached value\n")
			validated = last_exp(s)
			arduino_cache_stat[s] = arduino_cache_stat[s] + 1
			arduino_total_fail_read = arduino_total_fail_read + 1
		}

		reply = ""
		if arduino_cache_stat[s] > conf.Analysis.Cache_age {
			validated = 0
			arduino_cache_stat[s] = 0
		}
		lock.Lock()
		if validated > 0 {
			inverted := int(1 / float32(validated) * 10000)
			arduino_exp_stat[s] = inverted
		}
		lock.Unlock()
		time.Sleep(time.Second * 2)
	}
	check := comm2_arduino("S?")
	lock.Lock()
	if !strings.Contains(check, "ok") { // check if the reply is what we asked
		log.Printf("Periodic check failed (%q)!\n", check)
		arduino_total_fail_read = arduino_total_fail_read + 1
	}
	lock.Unlock()
	Alock.Lock()
	if TurnAlarm {
		a := comm2_arduino("A!")
		log.Printf("%s\n", a)
		TurnAlarm = false
	}
	Alock.Unlock()
	flush_serial()
}

func comm2_arduino(sensor string) (output string) {
	if conf.Serial.Tty == "none" {
		return "null"
	}
	sensor = sensor + "\n"
	lock.Lock()
	arduino_connected = false
	lock.Unlock()
	c := &serial.Config{Name: conf.Serial.Tty, Baud: conf.Serial.Baud, ReadTimeout: time.Millisecond * time.Duration(conf.Serial.Timeout)}
	s, err := serial.OpenPort(c)
	if err != nil {
		log.Printf("cannot open serial: %s\n", err)
		return "null"
	}
	reg, _ := regexp.Compile("[0-9]+")
	starts := time.Now()
	_, err = s.Write([]byte(sensor))
	if err != nil {
		log.Printf("cannot write to serial: %s\n", err)
		return "null"
	}
	if conf.Verbose {
		log.Printf("Asked: %s", sensor)
	}
	buf := []byte("________")
	nbytes, failed := s.Read(buf)
	t := time.Now()
	elapsed := t.Sub(starts)
	if nbytes < 3 {
		_, failed = s.Read(buf)
	}
	if failed != nil {
		log.Printf("cannot read from serial: %s\n", failed)
		arduino_total_fail_read = arduino_total_fail_read + 1
		output = "null"
	} else {
		whole_reply := string(buf)
		if conf.Verbose {
			log.Printf("Got %d bytes: %s, took %f", nbytes, whole_reply, elapsed.Seconds())
		}
		asked := strings.Split(sensor, "")
		if strings.HasPrefix(whole_reply, asked[0]) { // check if the reply is what we asked
			if asked[0] == "S" {
				lock.Lock()
				arduino_comm_time = float64(elapsed.Seconds())
				arduino_connected = true
				lock.Unlock()
				return "ok"
			}
			if asked[0] == "A" {
				return "ok"
			}
			number := reg.FindAllString(whole_reply, 1)
			output = number[0]
		} else {
			log.Printf("Unexpected reply\n")
			arduino_total_fail_read = arduino_total_fail_read + 1
			output = "null"
		}
	}
	s.Close()
	return output
}

func flush_serial() {
	if conf.Serial.Tty == "none" {
		return
	}
	c := &serial.Config{Name: conf.Serial.Tty, Baud: conf.Serial.Baud, ReadTimeout: time.Millisecond * time.Duration(conf.Serial.Timeout)}
	s, err := serial.OpenPort(c)
	if err != nil {
		log.Printf("%s\n", err)
		arduino_connected = false
		return
	}
	buf := make([]byte, 16)
	_, _ = s.Read(buf)
	s.Close()
}
