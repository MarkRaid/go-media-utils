// TODO: добавить проверку того, что ffmpeg вообще существует
// TODO: что делать, если мы передаём в names.txt абсолютный путь или если мы в командной строке передаём абсолютный путь
// TODO: надо короче, чтобы горутина режущая возвращала бы ошибку или nil в канал errors или done
// 		 тогда у нас уже имеется горутина, которая опустошает канал done - в ней нужно описать логику логирования
// TODO: катя просила, чтобы можно было по конкретной картинке найти кадр в плеере и сделать нормальный, не смазанный кадр в сцене
// TODO: перенести всё по пакетам
// TODO: добавить логирование
// TODO: переходим на git-flow с ветками, ветку мастер держим всегда для продакшена

// TODO: Заняться документацией, а то потом не понятно что это и как это использовать

package main

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/malashin/ffinfo"
	"github.com/urfave/cli/v2"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"

	"github.com/MarkRaid/go-media-utils/pkg/filenames"
	"github.com/MarkRaid/go-media-utils/pkg/fshelp"
	"github.com/MarkRaid/go-media-utils/pkg/mediahelp"
)

// go run framecut.go -i /home/mark/grive/Projects/Go/ffmpeg-screenshots/input -o /home/mark/grive/Projects/Go/ffmpeg-screenshots/output/ --names /home/mark/grive/Projects/Go/ffmpeg-screenshots/names.txt
// go run framecut.go -i ./input -o ./output/ - \
// "Я.Железо - тестирование устройств-1Oy_dp1drzI (2-я копия).mp4" \
// "Я.Железо - тестирование устройств-1Oy_dp1drzI (1-я копия).mp4" \
// "Я.Железо - тестирование устройств-1Oy_dp1drzI.mp4"

type cliFlagsValues struct {
	InputPath       string
	OutputPath      string
	ScreenshotCount int
	Extention       string
	MaxOffset       time.Duration
	FileList        []string
	GlobList        []string
	NotGlobList     []string
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
			Destination: &flagsValues.InputPath,
		},
		&cli.PathFlag{
			Name:        "output",
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
			Value:       5 * time.Minute,
			Destination: &flagsValues.MaxOffset,
		},
		&cli.PathFlag{
			Name:  "batch-file",
			Usage: "File with file paths, like a names.txt",
		},
		&cli.StringSliceFlag{
			Name:        "glob",
			Usage:       "Regular expressions that filter the list of files. It can be specified several times",
			Value:       cli.NewStringSlice("*"),
			DefaultText: "[\"*\"]",
		},
		&cli.StringSliceFlag{
			Name:        "notGlob",
			Usage:       "Regular expressions that filter the list of files. It can be specified several times",
			DefaultText: "[]",
		},
	}

	cliApp.Action = func(ctx *cli.Context) (err error) {
		if ctx.IsSet("batch-file") {
			flagsValues.FileList, err = fshelp.ReadFileList(ctx.String("batch-file"))

			if err != nil {
				return cli.Exit(err, 1)
			}

			if len(flagsValues.FileList) == 0 {
				return cli.Exit("File specified in \"batch-file\" flag is empty", 1)
			}
		} else {
			if !ctx.Args().Present() {
				return cli.Exit("You didn't specify any files as a positional argument, or you didn't specify \"name\" flag", 1)
			}

			flagsValues.FileList = ctx.Args().Slice()
		}

		allPatterns := make([]string, 10)
		allPatterns = append(allPatterns, ctx.StringSlice("glob")...)
		allPatterns = append(allPatterns, ctx.StringSlice("notGlob")...)

		if invalidPattern, err := filenames.TestPatternSlice(allPatterns); err != nil {
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
	videoDuration float64
	progress      *mpb.Progress
	filePath      string
	outFmtTempl   string
	numFrames     int
	maxOffset     time.Duration
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

	files, err := filenames.FilterByGlobs(
		flagsValues.FileList,
		flagsValues.GlobList,
		flagsValues.NotGlobList,
	)

	if err != nil {
		log.Fatal(err)
	}

	jobs := make(chan cutJob, 100)
	done := make(chan bool, 100)

	for id := 1; id <= 10; id++ {
		go cuttingWorker(jobs, done)
	}

	progress := mpb.New(
		mpb.WithWidth(60),
		mpb.WithRefreshRate(300*time.Millisecond),
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
		for range done {
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

		if !mediahelp.IsVideoFile(ffprobeData) {
			bar.Increment()
			// log.Fatalf("For file: %v, video stream not found", fileName, err.Error())
			continue
		}

		outFmtTempl := filepath.Join(
			flagsValues.OutputPath,
			iterDir,
			filepath.Base(strings.TrimSuffix(filePath, filepath.Ext(filePath))),
			fmt.Sprintf("frame_%%s.%s", flagsValues.Extention),
		)

		err = os.MkdirAll(
			filepath.Dir(outFmtTempl),
			os.FileMode(0775),
		)

		if err != nil {
			bar.Increment()
			log.Fatal(err)
		}

		stream := mediahelp.GetStream(ffprobeData, 0, mediahelp.StreamVideo)

		if stream == nil {
			// log.Fatalf("Could not get a stream type %s by index %d", string(mediahelp.StreamVideo), 0)
			continue
		}

		videoDuration, err := ffprobeData.StreamDuration(stream.Index)

		if err != nil && videoDuration <= 0 {
			// TODO: Тут надо просто написать, что ошибку в лог
			continue
		}

		job := cutJob{
			videoDuration: videoDuration,
			progress:      progress,
			filePath:      filePath,
			outFmtTempl:   outFmtTempl,
			numFrames:     flagsValues.ScreenshotCount,
			maxOffset:     flagsValues.MaxOffset,
		}

		jobs <- job
	}

	close(jobs)
	progress.Wait()
	close(done)
}

func cuttingWorker(jobs <-chan cutJob, done chan<- bool) {
	for job := range jobs {
		bar := job.progress.AddBar(int64(job.numFrames),
			mpb.BarFillerTrim(),
			mpb.BarRemoveOnComplete(),
			mpb.PrependDecorators(decor.Name("Сutting video:", decor.WCSyncSpaceR)),
			mpb.AppendDecorators(
				decor.NewPercentage(" %d ", decor.WCSyncWidth),
				decor.Name(filepath.Base(job.filePath), decor.WCSyncSpaceR),
			),
		)

		baseOffset := int(job.videoDuration) / (job.numFrames + 1)
		maxRandOffset := int(job.maxOffset.Seconds())

		if baseOffset < maxRandOffset {
			maxRandOffset = baseOffset
		}

		for iterNum := 1; iterNum <= job.numFrames; iterNum++ {
			randOffset := 0.0

			for i := 0; i < maxRandOffset; i++ {
				randOffset += rand.Float64()
			}

			ssOffset := float64(baseOffset*iterNum) + randOffset

			absoluteOutputFileName := fmt.Sprintf(
				job.outFmtTempl,
				Zfill(iterNum, job.numFrames),
			)

			// TODO: сама нарезка параллельна только на уровне файлов, что если сделать нарезку параллельной на самом файле
			// 		 можно попробовать запускать некоторое количество процессов exec не дожидаясь их завершения сразу, в этом цикле
			//       можно ввести ещё одни пулл, в котором сам файл будет нарезаться на кусочки процессами ffmpeg-а
			//       и где-то, после того, как процесс ffmpeg-а нарезал скриншот, мы будем ожидать его с помощью exec и инкрементировать прогресс бар
			// TODO: "0:v:0" - последнюю цифру нужно сделать параметром этой функции
			cmd := exec.Command("ffmpeg", "-hide_banner", "-n", "-ss", strconv.FormatInt(int64(ssOffset), 10), "-i", job.filePath, "-map", "0:v:0", "-vf", "\"scale=iw*sar:ih,setsar=1/1\"", "-sws_flags", "sinc", "-frames:v", "1", "-q:v", "1", "-f", "image2", "-update", "1", absoluteOutputFileName)

			// cmd.Stdout = os.Stdout
			// cmd.Stderr = os.Stderr
			// cmd.Stdin = os.Stdin

			if err := cmd.Run(); err != nil {
				log.Fatal(err)
			}

			bar.Increment()
		}

		done <- true
	}
}

func DigitCount(n int) int {
	return int(math.Ceil(math.Log10(math.Abs(float64(n)) + 0.5)))
}

func Zfill(num, maxNum int) string {
	return fmt.Sprintf("%0[2]*[1]d", num, DigitCount(maxNum))
}
