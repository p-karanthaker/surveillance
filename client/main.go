package main

import (
	"bytes"
	"flag"
	"log"
	"net/http"
	"time"

	"gocv.io/x/gocv"
)

var (
	frames     chan gocv.Mat
	window     gocv.Window
	fps        float64
	width      int
	height     int
	clipLength time.Duration
)

var BOUNDARY = []byte("--MJPEGBOUNDARY")
var CRLF_X2 = []byte("\r\n\r\n")

func main() {
	host := flag.String("host", "http://0.0.0.0:3000", "where to get stream from ip:port")
	flag.Float64Var(&fps, "fps", 30, "fps of the video")
	flag.IntVar(&width, "w", 640, "video width")
	flag.IntVar(&height, "h", 480, "video height")
	flag.DurationVar(&clipLength, "clip", 1*time.Hour, "Duration each video clip will be")
	onScreen := flag.Bool("onscreen", false, "display video on screen otherwise save locally")
	flag.Parse()

	frames = make(chan gocv.Mat, 30) // buffer up to 30 frames (probably doesn't need a buffer that large)

	resp, err := http.Get(*host)
	if err != nil {
		log.Fatalln(err)
	}
	defer resp.Body.Close()

	// start writer/display - will block until frames start arriving in channel
	if !*onScreen {
		go writeLocal()
	} else {
		window = *gocv.NewWindow("Live Camera View")
		go displayOnScreen()
	}

	parseResponse(resp)
}

func parseResponse(resp *http.Response) {
	b := make([]byte, 0, 512)
	for {
		n, err := resp.Body.Read(b[len(b):cap(b)])
		if err != nil {
			log.Fatalln("Error reading stream", err)
		}
		b = b[:len(b)+n]

		// find boundary
		boundaryN := bytes.Index(b, BOUNDARY)
		if boundaryN != -1 {
			for {
				if len(b) == cap(b) {
					b = append(b, 0)[:len(b)] // Add more capacity (let append pick how much).
				}
				n, err := resp.Body.Read(b[len(b):cap(b)])
				if err != nil {
					log.Fatalln("Error reading stream", err)
				}
				b = b[:len(b)+n]

				// keep reading until we find the next boundary
				// <-n+len(n)-><--index in this range--> searching for index in a slice of the buffer
				// [--BOUNDARY(......................)]
				nextBoundaryN := bytes.Index(b[boundaryN+len(BOUNDARY):], BOUNDARY)
				if nextBoundaryN != -1 {
					nextBoundaryN += len(BOUNDARY)                          // add length of boundary to index to get correct index in entire buffer
					END_OF_HEADER := bytes.Index(b, CRLF_X2) + len(CRLF_X2) // Headers must separate the body with a CRLFCRLF https://datatracker.ietf.org/doc/html/rfc9112#section-2.1
					frame := b[END_OF_HEADER:nextBoundaryN]                 // extract jpeg between boundaries
					go frameDecode(frame)
					b = b[nextBoundaryN:] // reset buffer so the next boundary is at the start of the buffer
					break
				}
			}
		}
	}
}

func frameDecode(frame []byte) {
	img, err := gocv.IMDecode(frame, gocv.IMReadColor)
	if err != nil {
		log.Fatalln("Error decoding frame", err)
	}
	if img.Empty() {
		return
	}
	frames <- img
}

func writeLocal() {
	for {
		saveFile := time.Now().Format("2006-01-02 15:04:05 Monday") + ".mkv"
		writer, err := gocv.VideoWriterFile(saveFile, "x264", fps, width, height, true)
		if err != nil {
			log.Fatalln("Error opening video writer device", saveFile)
		}
		for frame := 0; frame < int(clipLength.Seconds())*int(fps); frame++ {
			if err != nil {
				log.Fatalln("Error decoding frame", err)
			}
			img := <-frames
			if img.Empty() {
				break
			}
			writer.Write(img)
			img.Close()
		}
		writer.Close()
	}
}

func displayOnScreen() {
	for {
		img := <-frames
		if img.Empty() {
			break
		}
		window.IMShow(img)
		img.Close()
		window.WaitKey(1)
	}
}
