package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

const FORWARD = 1
const REVERSE = 0
const PAN = 1
const TILT = 0

const BLUE_LED = "/sys/class/gpio/gpio76/value"
const YELLOW_LED = "/sys/class/gpio/gpio77/value"

var MOTORD_FOLDER = "."
var EVENT_FILE = "event"
var PORT = 8090

// spaHandler implements the http.Handler interface, so we can use it
// to respond to HTTP requests. The path to the static directory and
// path to the index file within that static directory are used to
// serve the SPA in the given static directory.
type spaHandler struct {
	staticPath string
	indexPath  string
}

// ServeHTTP inspects the URL path to locate a file within the static dir
// on the SPA handler. If a file is found, it will be served. If not, the
// file located at the index path on the SPA handler will be served. This
// is suitable behavior for serving an SPA (single page application).
func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// get the absolute path to prevent directory traversal
	path, err := filepath.Abs(r.URL.Path)
	if err != nil {
		// if we failed to get the absolute path respond with a 400 bad request
		// and stop
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// prepend the path with the path to the static directory
	path = filepath.Join(h.staticPath, path)

	// check whether a file exists at the given path
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		// file does not exist, serve index.html
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	} else if err != nil {
		// if we got an error (that wasn't that the file doesn't exist) stating the
		// file, return a 500 internal server error and stop
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// otherwise, use http.FileServer to serve the static dir
	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
}

func miioMotorMove(motor string, direction string, steps string) {
	f, err := os.Create(MOTORD_FOLDER + "/" + EVENT_FILE)
	if err != nil {
		fmt.Println(err)
		return
	}
	_, err = f.WriteString(motor + " " + direction + " " + steps)
	if err != nil {
		fmt.Println(err)
		f.Close()
		return
	}
	err = f.Close()
	if err != nil {
		fmt.Println(err)
		return
	}
}

func miioMotorGoto(hor string, ver string) {
	f, err := os.Create(MOTORD_FOLDER + "/" + EVENT_FILE)
	if err != nil {
		fmt.Println(err)
		return
	}
	_, err = f.WriteString("goto " + hor + " " + ver)
	if err != nil {
		fmt.Println(err)
		f.Close()
		return
	}
	err = f.Close()
	if err != nil {
		fmt.Println(err)
		return
	}
}

func readPositionStatus() int {
	dat, err := ioutil.ReadFile(MOTORD_FOLDER + "/" + "status")
	check(err)
	s := string(dat[0])
	i, err := strconv.Atoi(s)
	return (i)
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func miioLedControl(led int, value int) {
	/*
		echo 1 > /sys/class/gpio/gpio36/value
		echo 0 > /sys/class/gpio/gpio36/value
		echo 1 > /sys/class/gpio/gpio78/value
		echo 0 > /sys/class/gpio/gpio78/value
	*/

	var valueString = strconv.Itoa(value)
	var data = []byte(valueString)

	if led == 1 {
		err := ioutil.WriteFile(YELLOW_LED, data, 0644)
		check(err)
	} else {
		err := ioutil.WriteFile(BLUE_LED, data, 0644)
		check(err)
	}
}

func ledControlRoute(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	var ledNum = 0
	var color = params["color"]
	var value = params["value"]

	if "yellow" == color {
		ledNum = 1
	}

	if value == "on" {
		miioLedControl(ledNum, 1)
	} else {
		miioLedControl(ledNum, 0)
	}

}

func motorMoveRoute(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	var motor = params["motor"]
	var direction = params["direction"]
	var steps = params["steps"]
	miioMotorMove(motor, direction, steps)
	var status = readPositionStatus()
	if status == 0 {
		fmt.Fprintf(w, "ok")
	} else {
		fmt.Fprintf(w, "overflow")
	}
}

func motorGotoRoute(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	var hor = params["hor"]
	var ver = params["ver"]
	miioMotorGoto(hor, ver)
	var status = readPositionStatus()
	if status == 0 {
		fmt.Fprintf(w, "ok")
	} else {
		fmt.Fprintf(w, "overflow")
	}
}

func getLocalIP() string {
	tt, err := net.Interfaces()
	check(err)
	for _, t := range tt {
		aa, err := t.Addrs()
		check(err)
		for _, a := range aa {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			v4 := ipnet.IP.To4()
			if v4 == nil || v4[0] == 127 { // loopback address
				continue
			}
			return v4.String()
		}
	}
	return ""
}

func printUsage() {
	programName := os.Args[0]
	fmt.Println("Usage: ")
	fmt.Printf("%v --motord_folder <path> --port <port>\n", programName)
}

func validateArgs() {
	argsWithoutProg := os.Args[1:]

	for index, value := range argsWithoutProg {
		if value == "--help" {
			printUsage()
			os.Exit(0)
		} else if value == "--motord_folder" {
			MOTORD_FOLDER = argsWithoutProg[index+1]
		} else if value == "--port" {
			var err error
			PORT, err = strconv.Atoi(argsWithoutProg[index+1])

			_ = PORT
			_ = err
		}
	}
}

func main() {

	validateArgs()

	if _, err := os.Stat(MOTORD_FOLDER + "/" + EVENT_FILE); os.IsNotExist(err) {
		fmt.Printf("%v file not found! \n", EVENT_FILE)
		panic("event file not found!")
	}

	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/motor_move/{motor}/{direction}/{steps}", motorMoveRoute).Methods("GET")
	router.HandleFunc("/motor_goto/{hor}/{ver}", motorGotoRoute).Methods("GET")
	router.HandleFunc("/led_control/{color}/{value}", ledControlRoute).Methods("GET")

	spa := spaHandler{staticPath: "static", indexPath: "index.html"}
	router.PathPrefix("/").Handler(spa)

	fmt.Printf("Server started at http://%s:%d \n", getLocalIP(), PORT)
	// var addr = "0.0.0.0:" + strconv.Itoa(PORT)
	srv := &http.Server{
		Handler: router,
		Addr:    "0.0.0.0:" + strconv.Itoa(PORT),
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
	// log.Fatal(http.ListenAndServe(":8080", router))
}
