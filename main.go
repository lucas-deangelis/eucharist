package main

import (
	"flag"
	"fmt"
	"hash/fnv"
	"net/http"
	"strconv"
	"sync"
	"text/template"
	"time"

	// Used for colorizing CLI output.
	"zgo.at/zli"
)

type printers struct {
	mu sync.Mutex

	l map[string]printer
}

type printer struct {
	// Channel to cancel a printing goroutine.
	done   chan struct{}
	period int
}

// Add a new printer if it does not exist for this string,
// and launch a goroutine that prints every `period` second.
func (p *printers) Add(s string, period int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Return early if we already have one printer for that string.
	if _, ok := p.l[s]; ok {
		return
	}

	ch := make(chan struct{})
	p.l[s] = printer{
		done:   ch,
		period: period,
	}
	go runPrinter(s, period, ch, stringToColor(s))
}

// Stop a printer if it exists for this string.
func (p *printers) Stop(s string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	printer, ok := p.l[s]
	if ok {
		printer.done <- struct{}{}
	}
}

type nameAndPeriod struct {
	Name   string
	Period int
}

// NamesAndPeriods return the names and periods of the printers.
func (p *printers) NamesAndPeriods() []nameAndPeriod {
	var s []nameAndPeriod
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range p.l {
		s = append(s, nameAndPeriod{
			Name:   k,
			Period: v.period,
		})
	}
	return s
}

// runPrinter creates a ticker that ticks every n seconds, and loops
// infinitely on either it or `ch`.
// If it received a tick, it prints `s` with a color, if it receives
// anything in the channel it stops.
func runPrinter(s string, n int, ch chan struct{}, color string) {
	ticker := time.NewTicker(time.Duration(int64(n)) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			printWithTime(s, color)
		case <-ch:
			return
		}
	}
}

var start = time.Now()

// printWithTime prints `s` prefix with the number of second since the start of the program.
func printWithTime(s, color string) {
	co := zli.ColorHex(color)
	fmt.Printf("%04.0f %s\n", time.Since(start).Seconds(), zli.Colorize(s, co))
}

// Flag variable to choose the port.
var port string

func main() {
	flag.StringVar(&port, "http", ":8080", "port")
	flag.Parse()

	myPrinters := printers{
		l: make(map[string]printer),
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Error parsing form data", http.StatusBadRequest)
				return
			}

			// If there's a "stop" at true, it means a "stop" button was clicked,
			// and thus we should try to stop a printer.
			stop := r.FormValue("stop")
			if stop == "true" {
				item := r.FormValue("item")
				if item != "" {
					myPrinters.Stop(item)
				}
				return
			}

			// If we don't have a "stop" at true, this is probably a request to add
			// a printer.
			toPrint := r.FormValue("text")
			if toPrint != "" {
				period, err := strconv.Atoi(r.FormValue("period"))
				if err != nil {
					period = 1
				}
				myPrinters.Add(toPrint, period)
			}

			// We render a partial template, the table, that will be switched out thanks to HTMX.
			if err := printersTemplate.Execute(w, myPrinters.NamesAndPeriods()); err != nil {
				http.Error(w, "Error rendering template", http.StatusInternalServerError)
			}
		} else {
			// If it's not a post we render the "main" template.
			if err := formTemplate.Execute(w, myPrinters.NamesAndPeriods()); err != nil {
				http.Error(w, "Error rendering template", http.StatusInternalServerError)
			}
		}
	})

	fmt.Printf("Server is listening on http://localhost%s\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		fmt.Printf("Failed to start server: %s\n", err)
	}
}

// Main template, with the form and the table.
var formTemplate = template.Must(template.New("form").Parse(`
<!DOCTYPE html>
<html>
<head>
    <title>Ticker</title>
</head>
<body>
    <form hx-boost="true">
        <label for="text">Text to print:</label><br>
        <input type="text" id="text" name="text" required><br>
		<label for="period">Every x seconds:</label><br>
		<input type="number" id="number" name="period" min="1" value="1" required> <br>
        <button hx-post="/" hx-target="#results">Launch a printer</button>
    </form>
	<div id="results">
		<table>
			<tr>
				<th>Name</th>
				<th>Period</th>
				<th></th>
			</tr>
		{{range .}}
			<tr>
				<td>{{.Name}}</td>
				<td>{{.Period}}</td>
				<td><button hx-post="/" hx-vals='{"item": "{{.}}", "stop": true}' hx-target="#results">Stop</button></td>
			</tr>
		{{end}}
		</table>
	</div>
	<script src="https://unpkg.com/htmx.org@1.9.2"
        integrity="sha384-L6OqL9pRWyyFU3+/bjdSri+iIphTN/bvYyM37tICVyOJkWZLpP2vGn6VUEXgzg6h"
        crossorigin="anonymous"></script>
</body>
</html>
`))

// "Partial" template, with only the table.
var printersTemplate = template.Must(template.New("numbers").Parse(`
<table>
<tr>
	<th>Name</th>
	<th>Period</th>
	<th></th>
</tr>
{{range .}}
<tr>
	<td>{{.Name}}</td>
	<td>{{.Period}}</td>
	<td><button hx-post="/" hx-vals='{"item": "{{.}}", "stop": true}' hx-target="#results">Stop</button></td>
</tr>
{{end}}
</table>
`))

// stringToColor takes a string, hashes it, and generates a bright color in hexadecimal format.
// The same string always results in the same color.
// Courtesy of GPT-4, including the comments except this line.
func stringToColor(input string) string {
	// Create a new FNV hasher
	hasher := fnv.New32()

	// Hash the input string
	_, err := hasher.Write([]byte(input))
	if err != nil {
		panic("Failed to hash input string")
	}

	// Get the hash value
	hash := hasher.Sum32()

	// Use bits of the hash to generate RGB values, ensuring each component is relatively bright.
	// We ensure the minimum value is 128 by adding 128 to the modulo result of 128, making the range 128-255.
	r := byte(hash&0xFF)%128 + 128
	g := byte((hash>>8)&0xFF)%128 + 128
	b := byte((hash>>16)&0xFF)%128 + 128

	// Return the color in hexadecimal format
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}
