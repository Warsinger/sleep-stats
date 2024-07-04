package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"image/color"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
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

func calculateNightlyStatistics(data map[string][]SleepData) map[string]map[string]time.Duration {
	nightlyStats := make(map[string]map[string]time.Duration)
	for date, entries := range data {
		stats := make(map[string]time.Duration)
		for _, entry := range entries {
			duration := entry.EndDate.Sub(entry.StartDate)
			stats[entry.Value] += duration
		}
		nightlyStats[date] = stats
	}
	return nightlyStats
}

func createPlot(nightlyStats map[string]map[string]time.Duration, useLines bool) {
	p := plot.New()

	p.Title.Text = "Sleep Statistics Over Time"
	p.X.Label.Text = "Date"
	p.Y.Label.Text = "Duration (min)"
	p.Legend.Top = true

	// Prepare data for plotting
	numTicks := len(nightlyStats)
	dates := make([]time.Time, 0, numTicks)
	inBedDurations := make([]float64, 0, numTicks)
	asleepCoreDurations := make([]float64, 0, numTicks)
	asleepREMDurations := make([]float64, 0, numTicks)
	asleepDeepDurations := make([]float64, 0, numTicks)
	awakeDurations := make([]float64, 0, numTicks)

	layout := "2006-01-02"
	for date, stats := range nightlyStats {
		dateParsed, _ := time.Parse(layout, date)
		dates = append(dates, dateParsed)
		inBedDurations = append(inBedDurations, stats["inBed"].Minutes())
		asleepCoreDurations = append(asleepCoreDurations, stats["asleepCore"].Minutes())
		asleepREMDurations = append(asleepREMDurations, stats["asleepREM"].Minutes())
		asleepDeepDurations = append(asleepDeepDurations, stats["asleepDeep"].Minutes())
		awakeDurations = append(awakeDurations, stats["awake"].Minutes())
	}

	// Sort dates for plotting
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	datePoints := make(plotter.XYs, len(dates))
	for i, date := range dates {
		datePoints[i].X = float64(date.Unix())
	}

	createItem := func(durations []float64, label string, color color.RGBA) plot.Plotter {
		points := make(plotter.XYs, len(dates))
		for i, duration := range durations {
			points[i].X = datePoints[i].X
			points[i].Y = duration
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
			scatter.GlyphStyle.Radius = vg.Points(2)
			item, thumb = scatter, scatter
		}
		p.Legend.Add(label, thumb)
		return item
	}

	p.Add(createItem(inBedDurations, "In Bed", color.RGBA{R: 255, G: 0, B: 0, A: 255}))
	p.Add(createItem(asleepCoreDurations, "Asleep Core", color.RGBA{R: 0, G: 255, B: 0, A: 255}))
	p.Add(createItem(asleepREMDurations, "Asleep REM", color.RGBA{R: 0, G: 0, B: 255, A: 255}))
	p.Add(createItem(asleepDeepDurations, "Asleep Deep", color.RGBA{R: 255, G: 255, B: 0, A: 255}))
	p.Add(createItem(awakeDurations, "Awake", color.RGBA{R: 128, G: 128, B: 128, A: 255}))

	p.X.Tick.Marker = plot.TimeTicks{Format: "2006-01"}

	if err := p.Save(15*vg.Inch, 8*vg.Inch, "sleep_statistics.svg"); err != nil {
		panic(err)
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
	nightlyStats := calculateNightlyStatistics(groupedData)

	createPlot(nightlyStats, *useLines)

	fmt.Println("Sleep Statistics by Date:")
	for date, stats := range nightlyStats {
		fmt.Printf("Date: %s\n", date)
		fmt.Printf("  Total In Bed: %v\n", stats["inBed"])
		fmt.Printf("  Total Asleep Core: %v\n", stats["asleepCore"])
		fmt.Printf("  Total Asleep REM: %v\n", stats["asleepREM"])
		fmt.Printf("  Total Asleep Deep: %v\n", stats["asleepDeep"])
		fmt.Printf("  Total Awake: %v\n", stats["awake"])
	}
}
