package main

import (
	"encoding/json"
	"fmt"
	"html/template" // Import for HTML templating
	"io/ioutil"
	"log"
	"net/http"
	"os" // Import os for environment variables (optional, but good practice for port)
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// These variables will be populated at build time using ldflags
// For local 'go run', they will retain their default values.
var (
	buildVersion = "v0.0.0-dev"  // Default version for local development
	buildCommit  = "local-build" // Default commit hash for local development
)

// Global Prometheus counter for ping requests
var pingCounter = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "ping_request_count",
		Help: "Number of requests handled by Ping handler",
	},
)

// PageData struct for passing data to the HTML template
type PageData struct {
	Title   string
	Version string // NEW: Field for version information
}

// Handler for the root path ("/") - serves the HTML page
func servePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tmpl, err := template.ParseFiles("index.html")
	if err != nil {
		log.Printf("Error parsing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// NEW: Pass version information to the template
	// Format the version string nicely: e.g., "v1.0.0 (abcdef1)"
	data := PageData{
		Title:   "Ping-Pong: Human, Robot, Heartbeat",
		Version: fmt.Sprintf("Version: %s (%s)", buildVersion, buildCommit),
	}
	err = tmpl.Execute(w, data)
	if err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// ---- ginkgo

// Define Ginkgo Report Structures (as defined previously)
type GinkgoReport struct {
	JobName            string     `json:"job_name"`
	TotalCasesNumber   int        `json:"total_cases_number"`
	PassedCasesNumber  int        `json:"passed_cases_number"`
	ErroredCasesNumber int        `json:"errored_cases_number"`
	FailedCasesNumber  int        `json:"failed_cases_number"`
	PassedCases        []TestCase `json:"passed_cases"`
	FailedCases        []TestCase `json:"failed_cases"`
	ErroredCases       []TestCase `json:"errored_cases"`
}

type TestCase struct {
	Title         string         `json:"title"`
	Status        string         `json:"status"`    // "passed", "failed", "errored", "skipped", "pending"
	TimeCost      string         `json:"time_cost"` // Keep as string, will parse to float64
	FailureReason *FailureReason `json:"failure_reason"`
}

type FailureReason struct {
	Message string `json:"message"`
}

// ------------------- Prometheus Metrics -------------------
var (
	// Using a mutex to protect concurrent metric updates
	metricsMutex = &sync.Mutex{}

	jobOverallStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ginkgo_job_overall_status",
			Help: "Overall status of the Ginkgo job (0=success, 1=failure).",
		},
		[]string{"job_name"},
	)

	totalCases = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ginkgo_total_cases",
			Help: "Total number of Ginkgo test cases run.",
		},
		[]string{"job_name"},
	)
	passedCases = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ginkgo_passed_cases",
			Help: "Number of Ginkgo test cases that passed.",
		},
		[]string{"job_name"},
	)
	failedCases = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ginkgo_failed_cases",
			Help: "Number of Ginkgo test cases that failed.",
		},
		[]string{"job_name"},
	)
	erroredCases = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ginkgo_errored_cases",
			Help: "Number of Ginkgo test cases that errored.",
		},
		[]string{"job_name"},
	)

	passRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ginkgo_pass_rate_percentage",
			Help: "Percentage of Ginkgo test cases that passed (Passed / Total * 100).",
		},
		[]string{"job_name"},
	)

	testCaseStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ginkgo_test_case_status",
			Help: "Status of individual Ginkgo test cases (1=status_type, 0=other).",
		},
		[]string{"job_name", "test_title", "status_type"},
	)

	testCaseTimeCost = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ginkgo_test_case_time_cost_seconds",
			Help: "Time cost of individual Ginkgo test cases in seconds.",
		},
		[]string{"job_name", "test_title"},
	)
)

// ------------------- Helper Functions -------------------

// calculatePercentage safely computes a percentage.
func calculatePercentage(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0.0
	}
	return (float64(numerator) / float64(denominator)) * 100.0
}

// cleanPrometheusLabel cleans a string to be suitable as a Prometheus label value.
// It removes non-alphanumeric characters (except underscore) and replaces spaces.
func cleanPrometheusLabel(s string) string {
	s = strings.ReplaceAll(s, "[It] ", "")
	s = strings.ReplaceAll(s, " ", "_")
	re := regexp.MustCompile(`[^a-zA-Z0-9_.]`) // Allow dots for file names, adjust as needed
	s = re.ReplaceAllString(s, "")
	s = strings.ToLower(s)
	return s
}

// updatePrometheusMetrics processes a GinkgoReport struct and updates Prometheus metrics.
func updatePrometheusMetrics(report GinkgoReport) {
	metricsMutex.Lock() // Protect metrics during update
	defer metricsMutex.Unlock()

	jobLabels := prometheus.Labels{"job_name": report.JobName}

	// Set overall job status
	if report.FailedCasesNumber > 0 || report.ErroredCasesNumber > 0 {
		jobOverallStatus.With(jobLabels).Set(1) // 1 for failure
	} else {
		jobOverallStatus.With(jobLabels).Set(0) // 0 for success
	}

	// Set overall counts
	totalCases.With(jobLabels).Set(float64(report.TotalCasesNumber))
	passedCases.With(jobLabels).Set(float64(report.PassedCasesNumber))
	failedCases.With(jobLabels).Set(float64(report.FailedCasesNumber))
	erroredCases.With(jobLabels).Set(float64(report.ErroredCasesNumber))

	// Calculate and set pass rate percentage
	rate := calculatePercentage(report.PassedCasesNumber, report.TotalCasesNumber)
	passRate.With(jobLabels).Set(rate)

	// Update individual test case metrics
	// First, reset all individual test case statuses for this job_name to 0
	// This ensures that tests that might have run in a previous report but not this one
	// are no longer showing as 1 (succeeded/failed/errored).
	// NOTE: This can be complex for high cardinality; consider if you truly need
	// per-test-case time-series for every single test run if tests come and go.
	// For consistent test suites, it's fine.

	// A more robust way to reset/manage individual test cases in a dynamic list is
	// to clear all metrics of a specific vector, but Prometheus clients don't
	// typically expose this easily without re-registering.
	// For simplicity, we'll just set the current state.

	// It's crucial to handle cases where a test might be missing from a subsequent report
	// without clearing all previous series. Prometheus's `expire_labels` or `staleness_delta`
	// might be useful on the Prometheus server side, or using a Pushgateway that clears metrics.

	// For this direct scrape model, new `job_name` labels create new series.
	// If the `job_name` is stable across runs, metrics for individual test cases
	// that no longer appear in the report will simply retain their last value.
	// If you want them to 'disappear' until next seen, you might need a custom collector
	// or a Pushgateway (which can delete specific series).

	// Clear existing individual test case metrics to avoid stale data
	// If you have many dynamic test_titles, this might be slow or not desired.
	// Consider if `testCaseStatus` and `testCaseTimeCost` should be `CounterVec` or `GaugeVec`
	// based on how you envision the data in Grafana. GaugeVec for latest status, CounterVec for increments.
	// Given the names "status" and "time_cost", GaugeVec is appropriate for the *latest* value.
	// For the exact same jobName and test_title, the gauge will simply be overwritten.

	updateIndividualTestCaseMetrics(report.JobName, report.PassedCases, "passed")
	updateIndividualTestCaseMetrics(report.JobName, report.FailedCases, "failed")
	updateIndividualTestCaseMetrics(report.JobName, report.ErroredCases, "errored")
	log.Printf("Metrics updated for job: %s", report.JobName)
}

func updateIndividualTestCaseMetrics(jobName string, cases []TestCase, statusType string) {
	for _, tc := range cases {
		cleanTitle := cleanPrometheusLabel(tc.Title)

		// Set status: 1 if this test case is of the given statusType, 0 otherwise
		// We re-set all status types to 0 first to ensure only one is 1 for a given test_title
		testCaseStatus.With(prometheus.Labels{"job_name": jobName, "test_title": cleanTitle, "status_type": "passed"}).Set(0)
		testCaseStatus.With(prometheus.Labels{"job_name": jobName, "test_title": cleanTitle, "status_type": "failed"}).Set(0)
		testCaseStatus.With(prometheus.Labels{"job_name": jobName, "test_title": cleanTitle, "status_type": "errored"}).Set(0)
		// For the current status type, set to 1
		testCaseStatus.With(prometheus.Labels{"job_name": jobName, "test_title": cleanTitle, "status_type": statusType}).Set(1)

		// Parse and set time cost
		timeCost, err := strconv.ParseFloat(tc.TimeCost, 64)
		if err != nil {
			log.Printf("Warning: [%s] Failed to parse time_cost for '%s': %v", jobName, tc.Title, err)
		} else {
			testCaseTimeCost.With(prometheus.Labels{"job_name": jobName, "test_title": cleanTitle}).Set(timeCost)
		}
	}
}

// ------------------- HTTP Handlers -------------------

// submitReportHandler handles POST requests to submit Ginkgo JSON reports.
func submitReportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	var report GinkgoReport
	err = json.Unmarshal(body, &report)
	if err != nil {
		log.Printf("Error unmarshalling JSON from request: %v", err)
		http.Error(w, "Invalid JSON format: "+err.Error(), http.StatusBadRequest)
		return
	}

	updatePrometheusMetrics(report)

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Report received and metrics updated for job: %s\n", report.JobName)
	log.Printf("Successfully processed report for job: %s at %s", report.JobName, time.Now().Format(time.RFC3339))
}

// ---- ginkgo

// The existing ping handler, now accessible via the web page
func ping(w http.ResponseWriter, req *http.Request) {
	// Increment the Prometheus counter for each ping request
	pingCounter.Inc()
	log.Println("Ping function invoked!") // Log to the server console

	fmt.Fprintf(w, "pong") // Send "pong" response to the client
}

func main() {
	// Register the Prometheus counter
	prometheus.MustRegister(pingCounter)

	// Register all Prometheus metrics
	prometheus.MustRegister(jobOverallStatus)
	prometheus.MustRegister(totalCases)
	prometheus.MustRegister(passedCases)
	prometheus.MustRegister(failedCases)
	prometheus.MustRegister(erroredCases)
	prometheus.MustRegister(passRate)
	prometheus.MustRegister(testCaseStatus)
	prometheus.MustRegister(testCaseTimeCost)

	// Configure HTTP routes
	http.HandleFunc("/submit-ginkgo-report", submitReportHandler)

	// Register HTTP handlers
	http.HandleFunc("/", servePage)             // Serves the HTML page at the root
	http.HandleFunc("/ping", ping)              // Your existing ping handler
	http.Handle("/metrics", promhttp.Handler()) // Prometheus metrics endpoint

	// Determine the port to listen on. Default to 8080 if not set by environment variable.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	log.Printf("Server starting on :%s (Version: %s)", port, fmt.Sprintf("%s (%s)", buildVersion, buildCommit))
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
