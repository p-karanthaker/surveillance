package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"net/http"
	"time"

	"github.com/p-karanthaker/surveillance/server/server"
	"gocv.io/x/gocv"
)

var (
	deviceID   int
	err        error
	webcam     *gocv.VideoCapture
	stream     *server.Stream
	frame      chan gocv.Mat
	frame2     chan gocv.Mat
	clipLength time.Duration
	width      int
	height     int
	fps        float64
	save       bool
)

func main() {
	flag.IntVar(&deviceID, "id", 0, "device id")
	host := flag.String("host", "0.0.0.0:3000", "where to serve from ip:port")
	codec := flag.String("codec", "MJPG", "MJPG or YUYV")
	flag.IntVar(&width, "w", 640, "video width")
	flag.IntVar(&height, "h", 480, "video height")
	flag.Float64Var(&fps, "fps", 30, "fps of the video")
	flag.DurationVar(&clipLength, "clip", 1*time.Hour, "Duration each video clip will be")
	flag.BoolVar(&save, "save", false, "whether to save locally or not")
	flag.Parse()

	frame = make(chan gocv.Mat, 30)
	frame2 = make(chan gocv.Mat, 30)

	webcam, err = gocv.OpenVideoCapture(deviceID)
	webcam.Set(gocv.VideoCaptureFOURCC, webcam.ToCodec(*codec))
	webcam.Set(gocv.VideoCaptureFrameWidth, float64(width))
	webcam.Set(gocv.VideoCaptureFrameHeight, float64(height))
	if err != nil {
		fmt.Printf("Error opening capture device: %v\n", deviceID)
		return
	}
	defer webcam.Close()

	stream = server.NewStream()

	go grabFrames()
	if save {
		go writeLocal()
	}
	go sendToStream()

	fmt.Println("Capturing. Point your browser to " + *host)

	http.Handle("/", stream)

	server := &http.Server{
		Addr: *host,
	}

	log.Fatal(server.ListenAndServe())
}

func writeLocal() {
	for {
		saveFile := time.Now().Format("2006-01-02 15:04:05 Monday") + ".mkv"
		writer, err := gocv.VideoWriterFile(saveFile, "x264", fps, width, height, true)
		if err != nil {
			log.Fatalln("Error opening video writer device", saveFile)
		}
		for frames := 0; frames < int(clipLength.Seconds())*int(fps); frames++ {
			if err != nil {
				log.Fatalln("Error decoding frame", err)
			}
			img := <-frame2
			if img.Empty() {
				break
			}
			writer.Write(img)
			img.Close()
		}
		go writer.Close()
	}
}

func sendToStream() {
	for {
		img := <-frame
		if img.Empty() {
			break
		}
		buf, _ := gocv.IMEncode(".jpg", img)
		stream.UpdateJPEG(buf.GetBytes())
		img.Close()
		buf.Close()
	}
}

func grabFrames() {
	for {
		img := gocv.NewMat()
		webcam.Read(&img)
		gocv.PutText(&img, time.Now().Format("2006-01-02 15:04:05"), image.Point{X: 0, Y: img.Rows() - 5}, gocv.FontHersheyDuplex, 1, color.RGBA{255, 255, 255, 0}, 1)
		if img.Empty() {
			continue
		}
		frame <- img
		if save {
			frame2 <- img.Clone()
		}
	}
}
