package main

import (
	"encoding/csv"
	"fmt"
	"image/color"
	"os"
	"sort"
	"strings"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

type SleepData struct {
	SourceName    string
	SourceVersion string
	ProductType   string
	Device        string
	StartDate     time.Time
	EndDate       time.Time
	Value         string
	TimeZone      string
}

func parseCSV(filename string) ([]SleepData, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Skip the first row (header)
	if _, err := reader.Read(); err != nil {
		return nil, err
	}

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var sleepData []SleepData
	for _, record := range records {
		if !strings.HasPrefix(record[3], "Watch") {
			continue // Skip non-watch entries
		}
		startDate, err := time.Parse("2006-01-02 15:04:05 -0700", record[5])
		if err != nil {
			return nil, err
		}
		endDate, err := time.Parse("2006-01-02 15:04:05 -0700", record[6])
		if err != nil {
			return nil, err
		}
		sleepData = append(sleepData, SleepData{
			SourceName:    record[1],
			SourceVersion: record[2],
			ProductType:   record[3],
			Device:        record[4],
			StartDate:     startDate,
			EndDate:       endDate,
			Value:         record[7],
			TimeZone:      record[8],
		})
	}
	return sleepData, nil
}

func groupByDate(data []SleepData) map[string][]SleepData {
	groupedData := make(map[string][]SleepData)
	for _, entry := range data {
		dateKey := entry.StartDate
		if entry.StartDate.Hour() < 12 {
			// Group with the previous day if the start time is before noon
			dateKey = dateKey.AddDate(0, 0, -1)
		}
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

func createPlot(nightlyStats map[string]map[string]time.Duration) {
	p := plot.New()

	p.Title.Text = "Sleep Statistics Over Time"
	p.X.Label.Text = "Date"
	p.Y.Label.Text = "Duration (min)"

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

	createLine := func(durations []float64, label string, color color.RGBA) *plotter.Line {
		points := make(plotter.XYs, len(dates))
		for i, duration := range durations {
			points[i].X = datePoints[i].X
			points[i].Y = duration
		}
		line, err := plotter.NewLine(points)
		if err != nil {
			panic(err)
		}
		line.LineStyle.Color = color
		line.LineStyle.Width = vg.Points(2)
		p.Legend.Add(label, line)
		p.Legend.Top = true
		return line
	}

	p.Add(createLine(inBedDurations, "In Bed", color.RGBA{R: 255, G: 0, B: 0, A: 255}))
	p.Add(createLine(asleepCoreDurations, "Asleep Core", color.RGBA{R: 0, G: 255, B: 0, A: 255}))
	p.Add(createLine(asleepREMDurations, "Asleep REM", color.RGBA{R: 0, G: 0, B: 255, A: 255}))
	p.Add(createLine(asleepDeepDurations, "Asleep Deep", color.RGBA{R: 255, G: 255, B: 0, A: 255}))
	p.Add(createLine(awakeDurations, "Awake", color.RGBA{R: 128, G: 128, B: 128, A: 255}))

	p.X.Tick.Marker = plot.TimeTicks{Format: "2006-01"}

	if err := p.Save(15*vg.Inch, 8*vg.Inch, "sleep_statistics.svg"); err != nil {
		panic(err)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide the CSV file as an argument.")
		os.Exit(1)
	}
	filename := os.Args[1]
	sleepData, err := parseCSV(filename)
	if err != nil {
		fmt.Printf("Error reading CSV file: %v\n", err)
		os.Exit(1)
	}

	groupedData := groupByDate(sleepData)
	nightlyStats := calculateNightlyStatistics(groupedData)

	createPlot(nightlyStats)

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
