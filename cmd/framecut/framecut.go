// TODO: добавить проверку того, что ffmpeg вообще существует
// TODO: что делать, если мы передаём в names.txt абсолютный путь или если мы в командной строке передаём абсолютный путь
//       тогда по

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sort"
	"math"
	"math/rand"
	"runtime"
	"time"
	"errors"
	"bufio"

	"github.com/urfave/cli/v2"
	"github.com/malashin/ffinfo"
    "github.com/vbauerster/mpb/v6"
    "github.com/vbauerster/mpb/v6/decor"
)

// go run framecut.go -i /home/mark/grive/Projects/Go/ffmpeg-screenshots/input -o /home/mark/grive/Projects/Go/ffmpeg-screenshots/output/ --names /home/mark/grive/Projects/Go/ffmpeg-screenshots/names.txt
// go run framecut.go -i /home/mark/grive/Projects/Go/ffmpeg-screenshots/input -o /home/mark/grive/Projects/Go/ffmpeg-screenshots/output/ - \
// "Я.Железо - тестирование устройств-1Oy_dp1drzI (2-я копия).mp4" \
// "Я.Железо - тестирование устройств-1Oy_dp1drzI (1-я копия).mp4" \
// "Я.Железо - тестирование устройств-1Oy_dp1drzI.mp4"

type StreamType string


const (
	StreamAny StreamType      = ""
	StreamVideo StreamType    = "video"
	StreamAudio StreamType    = "audio"
	StreamSubtitle StreamType = "subtitle"
)


type cliFlagsValues struct {
	InputPath        string
	OutputPath       string
	ScreenshotCount  int
	Extention        string
	MaxOffset        time.Duration
	FileList         []string
	GlobList         []string
	NotGlobList      []string
}


// TODO: тут ещё нужно подумать посидеть
func initCliApp(cliApp *cli.App) *cliFlagsValues {
	var flagsValues cliFlagsValues

	cliApp.Name = "Screenshots by FFMpeg"
	cliApp.Usage = "CLI for get screenshots"
	cliApp.Version = "0.1.1"

	cliApp.Flags = []cli.Flag{
		&cli.PathFlag{
			Name:        "input",
			Aliases:     []string{"i"},
			Usage:       "Video folder",
			Required:    true,
			Destination: &flagsValues.InputPath,
		},
		&cli.PathFlag{
			Name:        "ouput",
			Aliases:     []string{"o"},
			Usage:       "Output folder",
			Required:    true,
			Destination: &flagsValues.OutputPath,
		},
		&cli.StringFlag{
			Name:        "ext",
			Value:       "png",
			DefaultText: "png",
			Usage:       "Frame container, e.g.: png, bmp, jpg, tiff, etc.",
			Destination: &flagsValues.Extention,
		},
		&cli.IntFlag{
			Name:        "count",
			Usage:       "Number of screenshots per video.",
			Aliases:     []string{"c"},
			Value:       10,
			DefaultText: "10",
			Destination: &flagsValues.ScreenshotCount,
		},
		&cli.DurationFlag{
			Name:        "maxOffset",
			Usage:       "",
			Value:       5 * time.Second,
			Destination: &flagsValues.MaxOffset,
		},
		&cli.PathFlag{
			Name:        "names",
			Usage:       "File with file paths",
		},
		&cli.StringSliceFlag{
			Name:        "glob",
			Usage:       "Regular expressions that filter the list of files. It can be specified several times",
			Value: 		 cli.NewStringSlice("*"),
			DefaultText: "[\"*\"]",
		},
		&cli.StringSliceFlag{
			Name:        "notGlob",
			Usage:       "Regular expressions that filter the list of files. It can be specified several times",
			DefaultText: "[]",
		},
	}

	cliApp.Action = func(ctx *cli.Context) (err error) {
		if ctx.IsSet("names") {
			flagsValues.FileList, err = readFileList(ctx.String("names"))
		   
		    if err != nil {
		        return cli.Exit(err, 1)
		    }

		    if len(flagsValues.FileList) == 0 {
		    	return cli.Exit("File specified in \"names\" flag is empty", 1)
		    }
		} else {
			if !ctx.Args().Present() {
				return cli.Exit("You didn't specify any files as a positional argument, and you didn't specify \"name\" flag", 1)
			}

			flagsValues.FileList = ctx.Args().Slice()
		}

		allPatterns := []string{}
		allPatterns = append(allPatterns, ctx.StringSlice("glob")...)
		allPatterns = append(allPatterns, ctx.StringSlice("notGlob")...)

		if invalidPattern, err := testPatternSlice(allPatterns); err != nil {
			return cli.Exit(fmt.Sprintf("Invalid pattern %v, with error: %v", invalidPattern, err), 1)
		}

		flagsValues.GlobList = ctx.StringSlice("glob")
		flagsValues.NotGlobList = ctx.StringSlice("notGlob")

		if ctx.Int("count") < 1 {
			return cli.Exit("\"count\" must be a natural number", 1)
		}

		return nil
    }

    return &flagsValues
}


type cutJob struct {
	ffprobeData *ffinfo.File
	progress	*mpb.Progress
	filePath 	string
	outFmtTempl string
	numFrames 	int
	maxOffset   time.Duration
}


func main() {
	// TODO: это что такое
	log.SetFlags(0)

	// TODO: Это надо куда-то переместить
	rand.Seed(time.Now().UTC().UnixNano())

	var cliApp = cli.NewApp()
	var flagsValues = initCliApp(cliApp)

	if err := cliApp.Run(os.Args); err != nil {
		log.Fatal("Flags initialization failed: %v", err)
	}

	files, err := filterByGlobs(
		flagsValues.FileList,
		flagsValues.GlobList,
		flagsValues.NotGlobList,
	)

	if err != nil {
		log.Fatal(err)
	}

	jobs := make(chan cutJob, 100)
	done := make(chan bool, 100)

	numWorkers := int(math.Min(
		float64(10),
		float64(runtime.NumCPU()),
	))

    for id := 1; id <= numWorkers; id++ {
        go cuttingWorker(jobs, done)
    }

	progress := mpb.New(
		mpb.WithWidth(60),
		mpb.WithRefreshRate(300 * time.Millisecond),
	)

	bar := progress.AddBar(int64(len(files)),
		mpb.BarFillerTrim(),
		mpb.BarFillerClearOnComplete(),
		mpb.PrependDecorators(decor.Name("Сutting progress:", decor.WCSyncSpaceR)),
		mpb.AppendDecorators(
			decor.OnComplete(decor.NewPercentage("%d ", decor.WCSyncWidth), "Done!"),
			decor.Name("", decor.WCSyncSpaceR),
		),
	)

	go func() {
	    for _ = range done {
	    	bar.Increment()
	    }
	}()

	startT := time.Now()
	iterDir := startT.Format("01.02__15_04_05")

	for _, fileName := range files {
		filePath := filepath.Join(flagsValues.InputPath, fileName)

		// Ошибка это просто вывод stderr
		ffprobeData, err := ffinfo.Probe(filePath) 
		if err != nil {
		    bar.Increment()
		    // Здесь принтуем в итоговый лог что произошла такая-то ошибка
		    // Тут скорее всего ffprobe уже напишет для какого файла ошибка
		    // log.Fatalf("For file: %v, ffprobe error: %v", fileName, err.Error())
		    continue
		}

		if !isVideoFile(ffprobeData) {
			bar.Increment()
			// log.Fatalf("For file: %v, video stream not found", fileName, err.Error())
			continue
		}

		outFmtTempl := filepath.Join(
			flagsValues.OutputPath,
			iterDir,
			filepath.Base(strings.TrimSuffix(filePath, filepath.Ext(filePath))),
			fmt.Sprintf("screenshot_%%s.%s", flagsValues.Extention),
		)

		err = os.MkdirAll(
			filepath.Dir(outFmtTempl),
			os.FileMode(0775),
		)

		if err != nil {
			bar.Increment()
			log.Fatal(err)
		}

		job := cutJob{
			ffprobeData: ffprobeData,
			progress:	 progress,
			filePath: 	 filePath,
			outFmtTempl: outFmtTempl,
			numFrames:   flagsValues.ScreenshotCount,
			maxOffset:   flagsValues.MaxOffset,
		}

        jobs <- job
    }

    close(jobs)

    progress.Wait()
}


func cuttingWorker(jobs <-chan cutJob, done chan<- bool) {
	for job := range jobs {
		stream := GetStream(job.ffprobeData, 0, StreamVideo)

		if stream == nil {
			log.Fatalf("Could not get a stream type %s by index %d", string(StreamVideo), 0)
		}

		videoDuration, err := job.ffprobeData.StreamDuration(stream.Index)
		
		if err != nil && videoDuration <= 0 {
			// TODO: Тут надо просто написать, что ошибку в лог
			done <- false
			continue
		}

		bar := job.progress.AddBar(int64(job.numFrames),
			mpb.BarFillerTrim(),
			mpb.BarRemoveOnComplete(),
			mpb.PrependDecorators(decor.Name("Сutting video:", decor.WCSyncSpaceR)),
			mpb.AppendDecorators(
				decor.NewPercentage(" %d ", decor.WCSyncWidth),
				decor.Name(filepath.Base(job.filePath), decor.WCSyncSpaceR),
			),
		)

		baseOffset := int(videoDuration) / (job.numFrames + 1)
		maxRandOffset := int(job.maxOffset.Seconds())

		if baseOffset < maxRandOffset {
			maxRandOffset = baseOffset
		}

		randOffset := float64(rand.Intn(maxRandOffset))

		for frameNum := 1; frameNum <= job.numFrames; frameNum++ {
			ssOffset := float64(baseOffset * frameNum) + randOffset

			absoluteOutputFileName := fmt.Sprintf(
				job.outFmtTempl,
				Zfill(frameNum, job.numFrames),
			)

			cmd := exec.Command("ffmpeg", "-hide_banner", "-n", "-ss", strconv.FormatInt(int64(ssOffset), 10), "-i", job.filePath, "-map", "0:v:0", "-frames:v", "1", "-q:v", "1", "-f", "image2", "-update", "1", absoluteOutputFileName)

			// cmd.Stdout = os.Stdout
			// cmd.Stderr = os.Stderr
			// cmd.Stdin = os.Stdin

			if err = cmd.Run(); err != nil {
				log.Fatal(err)
			}

			bar.Increment()
		}

		done <- true
	}
}


func GetStream(ffprobeData *ffinfo.File, sIndex int, sType StreamType) *ffinfo.Stream {
	var streams []*ffinfo.Stream

	for _, stream := range ffprobeData.Streams {
		if stream.CodecType == string(sType) {
			streams = append(streams, &stream)
		}
	}

	sort.Slice(streams, func(i, j int) (less bool) {
		s_i := streams[i]
		s_j := streams[j]

		return s_i.Index < s_j.Index
	})

	if len(streams) - 1 < sIndex {
		return nil
	}

	return streams[sIndex]
}


func isVideoFile(ffprobeData *ffinfo.File) bool {
	for _, stream := range ffprobeData.Streams {
		if stream.CodecType == string(StreamVideo) {
			return true
		}
	}

	return false
}


func filterByGlobs(names, globs, notGlobs []string) (matched []string, err error) {
   	for _, name := range names {
   		if matchedWithAnyGlob(name, globs) && !matchedWithAnyGlob(name, notGlobs) {
   			matched = append(matched, name)
   		}
   	}

    if len(matched) != 0 {
    	return matched, nil
    }

    for _, name := range names {
	    if matchedWithAnyGlob(name, globs) {
			return matched, errors.New(
				"All names that match at least one glob also match at least one notGlobs",
			)
	    }
    }

	return matched, errors.New("No names matching glob expressions")
}


func matchedWithAnyGlob(name string, globs []string) bool {
	for _, pattern := range globs {
   		if matched, _ := filepath.Match(pattern, name); matched {   			
			return true
		}
	}

	return false
}


func DigitCount(n int) int {
	return int(math.Ceil(math.Log10(math.Abs(float64(n)) + 0.5)))
}


func Zfill(num, maxNum int) string {
	return fmt.Sprintf("%0[2]*[1]d", num, DigitCount(maxNum))
}


func readFileList(path string) (lines []string, err error) {
    file, err := os.Open(path)

    if err != nil {
        return
    }

    defer file.Close()

    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        lines = append(lines, scanner.Text())
    }

    if err = scanner.Err(); err != nil {
        return
    }

    return
}


func testPatternSlice(patterns []string) (pattern string, err error) {
	for _, pattern = range patterns {
		if _, err = filepath.Match(pattern, ""); err != nil {
			return
		}
	}

	return "", nil
}