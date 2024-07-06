package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"image/color"
	"io"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"golang.org/x/exp/maps"
	"gonum.org/v1/gonum/stat"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
)

type SleepData struct {
	StartDate time.Time
	EndDate   time.Time
	Value     string
}

func parseCSV(filename string, startFilter, endFilter *time.Time) ([]SleepData, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)

	// check for the "sep=" starting line and if it exists read past it before parsing CSV
	// TODO go ahead and read the separator character and use it for the CSV delim
	head, err := reader.Peek(4)
	if err != nil {
		return nil, err

	}
	if string(head) == "sep=" {
		_, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
	}

	csvReader := csv.NewReader(reader)

	// read and parse the first row
	header, err := csvReader.Read()
	if err != nil {
		return nil, err
	}
	headerMap := parseHeader(header)

	var sleepData []SleepData
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// Skip non-watch entries
		isWatch := strings.HasPrefix(record[headerMap["productType"]], "Watch")
		if !isWatch {
			continue
		}

		startDate, err := time.Parse("2006-01-02 15:04:05 +0000", record[headerMap["startDate"]])
		if err != nil {
			return nil, err
		}
		endDate, err := time.Parse("2006-01-02 15:04:05 +0000", record[headerMap["endDate"]])
		if err != nil {
			return nil, err
		}
		if (startFilter == nil || startDate.After(*startFilter) || startDate.Equal(*startFilter)) &&
			(endFilter == nil || endDate.Before(*endFilter) || endDate.Equal(*endFilter)) {
			sleepData = append(sleepData, SleepData{
				StartDate: startDate,
				EndDate:   endDate,
				Value:     record[headerMap["value"]],
			})
		}
	}
	return sleepData, nil
}

// parse the header names and return a map of the names to the index
func parseHeader(header []string) map[string]int {
	headerMap := make(map[string]int, (len(header)))
	for i, name := range header {
		headerMap[name] = i
	}
	fmt.Println(headerMap)
	return headerMap
}

func groupByDate(data []SleepData) map[string][]SleepData {
	groupedData := make(map[string][]SleepData)
	for _, entry := range data {
		dateKey := entry.StartDate
		// don't need to account for date spanning since the data is in UTC
		// if entry.StartDate.Hour() < 12 {
		// 	// Group with the previous day if the start time is before noon
		// 	dateKey = dateKey.AddDate(0, 0, -1)
		// }
		dateKeyStr := dateKey.Format("2006-01-02")
		groupedData[dateKeyStr] = append(groupedData[dateKeyStr], entry)
	}
	return groupedData
}

func calculateNightlyStatistics(data map[string][]SleepData) (map[string]map[string]time.Duration, map[string]int) {
	nightlyStats := make(map[string]map[string]time.Duration)
	awakeCount := make(map[string]int)
	for date, entries := range data {
		stats := make(map[string]time.Duration)
		var count int = 0
		for _, entry := range entries {
			duration := entry.EndDate.Sub(entry.StartDate)
			stats[entry.Value] += duration
			if entry.Value == "inBed" {
				count++
			}
		}
		nightlyStats[date] = stats
		awakeCount[date] = count
	}
	return nightlyStats, awakeCount
}

func createPlot(nightlyStats map[string]map[string]time.Duration, awakeCount map[string]int, useLines bool) {
	p := plot.New()

	p.Title.Text = "Sleep Statistics Over Time"
	p.X.Label.Text = "Date"
	p.Y.Label.Text = "Duration (hours)"
	p.Y.Scale = plot.LogScale{}
	p.Legend.Top = true

	// Prepare data for plotting
	numTicks := len(nightlyStats)
	dates := make([]time.Time, 0, numTicks)
	inBedDurations := make([]float64, 0, numTicks)
	asleepCoreDurations := make([]float64, 0, numTicks)
	asleepREMDurations := make([]float64, 0, numTicks)
	asleepDeepDurations := make([]float64, 0, numTicks)
	awakeDurations := make([]float64, 0, numTicks)
	awakeCountPlot := make([]float64, 0, numTicks)
	datePoints := make(plotter.XYs, numTicks)

	layout := "2006-01-02"
	for date := range nightlyStats {
		dateParsed, _ := time.Parse(layout, date)
		dates = append(dates, dateParsed)
	}

	// Sort dates for plotting
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	for i, date := range dates {
		// fmt.Printf("%d, %s", i, date)
		datePoints[i].X = float64(date.Unix())
	}

	for _, date := range dates {
		dateKey := date.Format(layout)
		stats := nightlyStats[dateKey]
		inBedDurations = append(inBedDurations, stats["inBed"].Hours())
		asleepCoreDurations = append(asleepCoreDurations, stats["asleepCore"].Hours())
		asleepREMDurations = append(asleepREMDurations, stats["asleepREM"].Hours())
		asleepDeepDurations = append(asleepDeepDurations, stats["asleepDeep"].Hours())
		awakeDurations = append(awakeDurations, stats["awake"].Hours())
		awakeCountPlot = append(awakeCountPlot, float64(awakeCount[dateKey]))
	}

	createItem := func(durations []float64, label string, color color.RGBA) []plot.Plotter {
		points := make(plotter.XYs, len(dates))
		for i, duration := range durations {
			points[i].X = datePoints[i].X
			if duration == 0 {
				points[i].Y = 0.01
			} else {
				points[i].Y = duration
			}
		}
		var item plot.Plotter
		var thumb plot.Thumbnailer

		if useLines {
			line, err := plotter.NewLine(points)
			if err != nil {
				panic(err)
			}
			line.LineStyle.Color = color
			line.LineStyle.Width = vg.Points(2)
			p.Legend.Add(label, line)
			item, thumb = line, line
		} else {
			scatter, err := plotter.NewScatter(points)
			if err != nil {
				panic(err)
			}
			scatter.GlyphStyle.Color = color
			scatter.GlyphStyle.Radius = vg.Points(3)
			scatter.GlyphStyle.Shape = draw.CircleGlyph{}
			item, thumb = scatter, scatter
		}
		p.Legend.Add(label, thumb)

		return []plot.Plotter{item, linearRegression(points, color)}
	}

	// p.Add(createItem(inBedDurations, "In Bed", color.RGBA{R: 255, G: 0, B: 0, A: 255})...)
	p.Add(createItem(asleepCoreDurations, "Core", color.RGBA{R: 0, G: 255, B: 0, A: 255})...)
	p.Add(createItem(asleepREMDurations, "REM", color.RGBA{R: 255, G: 0, B: 255, A: 255})...)
	p.Add(createItem(asleepDeepDurations, "Deep", color.RGBA{R: 0, G: 122, B: 122, A: 255})...)
	p.Add(createItem(awakeDurations, "Awake", color.RGBA{R: 128, G: 128, B: 128, A: 255})...)
	// p.Add(createItem(awakeCountPlot, "Awake Count", color.RGBA{R: 255, G: 155, B: 156, A: 255})...)

	p.X.Tick.Marker = plot.TimeTicks{Format: "2006-01"}

	if err := p.Save(15*vg.Inch, 8*vg.Inch, "sleep_statistics.svg"); err != nil {
		panic(err)
	}
}

func linearRegression(points plotter.XYs, color color.RGBA) plot.Plotter {
	var (
		xs      = make([]float64, len(points))
		ys      = make([]float64, len(points))
		weights []float64
	)

	for i := range xs {
		xs[i] = points[i].X
		ys[i] = points[i].Y
	}

	// y = alpha + beta*x
	alpha, beta := stat.LinearRegression(xs, ys, weights, false)

	rPoints := make(plotter.XYs, len(xs))
	lineFunc := func(x float64) float64 {
		return alpha + beta*x
	}

	for i := range xs {
		rPoints[i].X = xs[i]
		rPoints[i].Y = lineFunc(xs[i])
	}

	rline, err := plotter.NewLine(rPoints)
	if err != nil {
		panic(err)
	}
	rline.LineStyle.Color = color
	rline.LineStyle.Width = vg.Points(2)
	return rline
}

func outputStats(nightlyStats map[string]map[string]time.Duration, awakeCount map[string]int) {
	fmt.Println("Sleep Statistics by Date:")

	dates := maps.Keys(nightlyStats)
	slices.Sort(dates)

	for _, date := range dates {
		stats := nightlyStats[date]
		fmt.Printf("%s\tBed: %v\tCore: %v\tREM: %v\tDeep: %v\tAwake: %v\tAwake Count: %v\n",
			date, stats["inBed"], stats["asleepCore"], stats["asleepREM"], stats["asleepDeep"], stats["awake"], awakeCount[date])
	}
}

func main() {
	filename := flag.String("file", "", "CSV file containing sleep data")
	start := flag.String("start", "", "Start date (inclusive) in YYYY-MM-DD format")
	end := flag.String("end", "", "End date (inclusive) in YYYY-MM-DD format")
	useLines := flag.Bool("lines", false, "whether to plot with lines, default to points")
	flag.Parse()

	if *filename == "" {
		fmt.Println("Please provide the CSV file as an argument.")
		os.Exit(1)
	}

	var startDate, endDate *time.Time
	if *start != "" {
		parsedStart, err := time.Parse("2006-01-02", *start)
		if err != nil {
			fmt.Printf("Invalid start date format: %v\n", err)
			os.Exit(1)
		}
		startDate = &parsedStart
	}
	if *end != "" {
		parsedEnd, err := time.Parse("2006-01-02", *end)
		if err != nil {
			fmt.Printf("Invalid end date format: %v\n", err)
			os.Exit(1)
		}
		endDate = &parsedEnd
	}
	sleepData, err := parseCSV(*filename, startDate, endDate)
	if err != nil {
		fmt.Printf("Error reading CSV file: %v\n", err)
		os.Exit(1)
	}

	groupedData := groupByDate(sleepData)
	nightlyStats, awakeCount := calculateNightlyStatistics(groupedData)

	createPlot(nightlyStats, awakeCount, *useLines)

	outputStats(nightlyStats, awakeCount)
}
