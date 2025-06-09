package testutil

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Color codes for terminal output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[1;31m"
	ColorGreen  = "\033[1;32m"
	ColorYellow = "\033[1;33m"
	ColorBlue   = "\033[1;34m"
	ColorPurple = "\033[1;35m"
	ColorCyan   = "\033[1;36m"
	ColorWhite  = "\033[1;37m"
	ColorGray   = "\033[0;37m"

	// Background colors
	BgBlack   = "\033[40m"
	BgRed     = "\033[41m"
	BgGreen   = "\033[42m"
	BgYellow  = "\033[43m"
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"
	BgWhite   = "\033[47m"
)

// ProgressReporter provides real-time progress updates for long-running tests
type ProgressReporter struct {
	mu            sync.Mutex
	writer        io.Writer
	testName      string
	startTime     time.Time
	lastUpdate    time.Time
	total         int
	current       int
	updateRate    time.Duration
	spinnerStates []string
	spinnerIndex  int
	isActive      bool
	stopChan      chan struct{}
	doneChan      chan struct{}
	hideRequests  bool
	quietMode     bool
	lastPercent   int
}

// NewProgressReporter creates a new progress reporter for a test
func (p *ProgressReporter) SetHideRequests(hide bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hideRequests = hide
}

// Start begins the progress reporting
func (p *ProgressReporter) Start() {
	p.mu.Lock()
	if p.isActive {
		p.mu.Unlock()
		return
	}
	p.isActive = true
	p.mu.Unlock()

	// Print initial status
	p.printNewLine(fmt.Sprintf("▶ %s%s%s: Starting...", ColorCyan, p.testName, ColorReset))

	go func() {
		defer close(p.doneChan)
		ticker := time.NewTicker(p.updateRate)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				p.update()
			case <-p.stopChan:
				p.clearLine()
				return
			}
		}
	}()
}

// Stop ends the progress reporting
func (p *ProgressReporter) Stop() {
	p.mu.Lock()
	if !p.isActive {
		p.mu.Unlock()
		return
	}
	p.isActive = false
	p.mu.Unlock()

	close(p.stopChan)
	<-p.doneChan

	// Print final status
	elapsed := time.Since(p.startTime)
	p.printNewLine(fmt.Sprintf("✓ %s%s%s: Completed in %s",
		ColorGreen, p.testName, ColorReset, formatDuration(elapsed)))
}

// Increment increases the current progress count
func (p *ProgressReporter) Increment() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current++
	p.lastUpdate = time.Now()

	// Print milestone updates
	newPercent := int(float64(p.current) * 100 / float64(p.total))
	if newPercent != p.lastPercent && newPercent%10 == 0 {
		p.lastPercent = newPercent
		if !p.quietMode {
			p.clearLine()
			fmt.Fprintf(p.writer, "%s⏳ %s: %d%% complete (%d/%d)%s\n",
				ColorYellow, p.testName, newPercent, p.current, p.total, ColorReset)
		}
	}
}

// IncrementBy increases the current progress count by the specified amount
func (p *ProgressReporter) IncrementBy(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current += n
	p.lastUpdate = time.Now()
}

// SetTotal updates the total number of items
func (p *ProgressReporter) SetTotal(total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.total = total
}

// GetCurrent returns the current progress count
func (p *ProgressReporter) GetCurrent() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}

// GetTotal returns the total progress count
func (p *ProgressReporter) GetTotal() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.total
}

// IsComplete returns true if progress is complete
func (p *ProgressReporter) IsComplete() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current >= p.total
}

// Message displays a message without affecting the progress bar
func (p *ProgressReporter) Message(format string, args ...interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Если скрываем детали запросов или в тихом режиме, то не выводим сообщения
	if p.hideRequests || p.quietMode {
		return
	}

	p.clearLine()
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(p.writer, "%s%s%s\n", ColorBlue, msg, ColorReset)
}

// printNewLine prints a message on a new line
func (p *ProgressReporter) printNewLine(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clearLine()
	fmt.Fprintf(p.writer, "%s\n", msg)
}

// update displays the current progress
func (p *ProgressReporter) update() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// В тихом режиме обновляем только каждые 5%
	if p.quietMode {
		newPercent := int(float64(p.current) * 100 / float64(p.total))
		if newPercent == p.lastPercent || (newPercent%5 != 0) {
			return
		}
		p.lastPercent = newPercent
	}

	p.clearLine()

	// Calculate progress percentage
	var percent float64
	if p.total > 0 {
		percent = float64(p.current) * 100 / float64(p.total)
	}

	// Calculate elapsed time
	elapsed := time.Since(p.startTime)
	elapsedStr := formatDuration(elapsed)

	// Calculate estimated time remaining
	var etaStr string
	if p.current > 0 && p.total > 0 {
		eta := time.Duration(float64(elapsed) * float64(p.total-p.current) / float64(p.current))
		etaStr = formatDuration(eta)
	} else {
		etaStr = "calculating..."
	}

	// Update spinner
	spinner := p.spinnerStates[p.spinnerIndex]
	p.spinnerIndex = (p.spinnerIndex + 1) % len(p.spinnerStates)

	// Create loading bar
	const barWidth = 30
	completedWidth := int(float64(barWidth) * float64(p.current) / float64(p.total))
	if completedWidth > barWidth {
		completedWidth = barWidth
	}

	bar := "["
	for i := 0; i < barWidth; i++ {
		if i < completedWidth {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	bar += "]"

	// Format and print the progress line
	if p.quietMode {
		// Simplified output for quiet mode
		fmt.Fprintf(p.writer, "%s %s: %s%.1f%%%s [%d/%d] %s\r",
			spinner, p.testName, ColorYellow, percent, ColorReset,
			p.current, p.total, etaStr)
	} else {
		// Full output
		fmt.Fprintf(p.writer, "%s %s %s%s%s %s%.1f%%%s [%s%d/%d%s] %s%s%s %s%s%s\r",
			spinner,
			p.testName,
			ColorGreen, bar, ColorReset,
			ColorYellow, percent, ColorReset,
			ColorCyan, p.current, p.total, ColorReset,
			ColorGray, elapsedStr, ColorReset,
			ColorGray, etaStr, ColorReset,
		)
	}
}

// clearLine clears the current line in the terminal
func (p *ProgressReporter) clearLine() {
	fmt.Fprint(p.writer, "\r\033[K") // Clear the line
}

// formatDuration formats a duration in a human-readable format
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// ProgressGroup manages multiple progress reporters
type ProgressGroup struct {
	mu        sync.Mutex
	reporters map[string]*ProgressReporter
	quietMode bool
}

// NewProgressGroup creates a new progress group
func NewProgressGroup() *ProgressGroup {
	return &ProgressGroup{
		reporters: make(map[string]*ProgressReporter),
		quietMode: false,
	}
}

// SetQuietMode sets quiet mode for all reporters
func (pg *ProgressGroup) SetQuietMode(quiet bool) {
	pg.mu.Lock()
	pg.quietMode = quiet
	for _, reporter := range pg.reporters {
		reporter.SetQuietMode(quiet)
	}
	pg.mu.Unlock()
}

// AddReporter adds a reporter to the group
func (pg *ProgressGroup) AddReporter(name string, reporter *ProgressReporter) {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	reporter.SetQuietMode(pg.quietMode)
	pg.reporters[name] = reporter
}

// GetReporter gets a reporter by name
func (pg *ProgressGroup) GetReporter(name string) *ProgressReporter {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	return pg.reporters[name]
}

// StopAll stops all reporters in the group
func (pg *ProgressGroup) StopAll() {
	pg.mu.Lock()
	reporters := make([]*ProgressReporter, 0, len(pg.reporters))
	for _, reporter := range pg.reporters {
		reporters = append(reporters, reporter)
	}
	pg.mu.Unlock()

	for _, reporter := range reporters {
		reporter.Stop()
	}
}

// SectionReporter reports progress for a test section
type SectionReporter struct {
	name        string
	startTime   time.Time
	writer      io.Writer
	hideDetails bool
	quietMode   bool
}

// NewSectionReporter creates a new section reporter
func NewSectionReporter(name string) *SectionReporter {
	return &SectionReporter{
		name:        name,
		startTime:   time.Now(),
		writer:      os.Stdout,
		hideDetails: true, // По умолчанию скрываем детали
		quietMode:   false,
	}
}

// SetQuietMode enables or disables quiet mode
func (s *SectionReporter) SetQuietMode(quiet bool) {
	s.quietMode = quiet
}

// SetHideDetails sets whether to hide detailed status messages
func (s *SectionReporter) SetHideDetails(hide bool) {
	s.hideDetails = hide
}

// Start begins the section
func (s *SectionReporter) Start() {
	if !s.quietMode {
		fmt.Fprintf(s.writer, "%s▶ %s%s\n", ColorBlue, s.name, ColorReset)
	}
}

// End completes the section
func (s *SectionReporter) End() {
	elapsed := time.Since(s.startTime)
	if !s.quietMode {
		fmt.Fprintf(s.writer, "%s✓ %s %s(%s)%s\n",
			ColorGreen, s.name, ColorGray, formatDuration(elapsed), ColorReset)
	}
}

// Fail marks the section as failed
func (s *SectionReporter) Fail(err error) {
	elapsed := time.Since(s.startTime)
	errMsg := ""
	if err != nil {
		errMsg = ": " + err.Error()
	}
	// Всегда показываем ошибки, даже в тихом режиме
	fmt.Fprintf(s.writer, "%s✗ %s%s %s(%s)%s\n",
		ColorRed, s.name, errMsg, ColorGray, formatDuration(elapsed), ColorReset)
}

// Status reports a status update for the section
func (s *SectionReporter) Status(format string, args ...interface{}) {
	// Если скрываем детали или в тихом режиме, то не выводим статусные сообщения
	if s.hideDetails || s.quietMode {
		return
	}

	msg := fmt.Sprintf(format, args...)
	elapsed := time.Since(s.startTime)
	fmt.Fprintf(s.writer, "%s  → %s %s(+%s)%s\n",
		ColorYellow, msg, ColorGray, formatDuration(elapsed), ColorReset)
}

// TestProgressLogger provides a simple logger for test progress
type TestProgressLogger struct {
	testName    string
	writer      io.Writer
	hideDetails bool
	quietMode   bool
}

// NewTestProgressLogger creates a new test progress logger
func NewTestProgressLogger(testName string) *TestProgressLogger {
	return &TestProgressLogger{
		testName:    testName,
		writer:      os.Stdout,
		hideDetails: true, // По умолчанию скрываем детали
		quietMode:   false,
	}
}

// SetQuietMode enables or disables quiet mode
func (l *TestProgressLogger) SetQuietMode(quiet bool) {
	l.quietMode = quiet
}

// SetHideDetails sets whether to hide detailed log messages
func (l *TestProgressLogger) SetHideDetails(hide bool) {
	l.hideDetails = hide
}

// Infof logs an informational message
func (l *TestProgressLogger) Infof(format string, args ...interface{}) {
	if l.hideDetails || l.quietMode {
		return
	}

	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.writer, "%s[INFO] %s: %s%s\n", ColorBlue, l.testName, msg, ColorReset)
}

// Warnf logs a warning message
func (l *TestProgressLogger) Warnf(format string, args ...interface{}) {
	if l.quietMode {
		return
	}

	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.writer, "%s[WARN] %s: %s%s\n", ColorYellow, l.testName, msg, ColorReset)
}

// Errorf logs an error message
func (l *TestProgressLogger) Errorf(format string, args ...interface{}) {
	// Всегда показываем ошибки, даже в тихом режиме
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.writer, "%s[ERROR] %s: %s%s\n", ColorRed, l.testName, msg, ColorReset)
}

// Successf logs a success message
func (l *TestProgressLogger) Successf(format string, args ...interface{}) {
	if l.quietMode {
		return
	}

	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.writer, "%s[SUCCESS] %s: %s%s\n", ColorGreen, l.testName, msg, ColorReset)
}

// Debugf logs a debug message
func (l *TestProgressLogger) Debugf(format string, args ...interface{}) {
	if l.hideDetails || l.quietMode {
		return
	}

	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.writer, "%s[DEBUG] %s: %s%s\n", ColorGray, l.testName, msg, ColorReset)
}

// FormatHeader formats a header for test output
func FormatHeader(title string) string {
	line := strings.Repeat("-", 80)
	return fmt.Sprintf("%s%s%s\n%s%s%s\n%s%s%s\n",
		ColorCyan, line, ColorReset,
		ColorCyan, title, ColorReset,
		ColorCyan, line, ColorReset)
}

// FormatFooter formats a footer for test output
func FormatFooter() string {
	line := strings.Repeat("-", 80)
	return fmt.Sprintf("%s%s%s\n", ColorCyan, line, ColorReset)
}
func NewProgressReporter(testName string, total int) *ProgressReporter {
	return &ProgressReporter{
		writer:        os.Stdout,
		testName:      testName,
		startTime:     time.Now(),
		total:         total,
		updateRate:    300 * time.Millisecond, // Faster updates
		spinnerStates: []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"},
		stopChan:      make(chan struct{}),
		doneChan:      make(chan struct{}),
		hideRequests:  true,
		quietMode:     false,
		lastPercent:   -1,
	}
}

// SetQuietMode enables or disables quiet mode (minimal output)
func (p *ProgressReporter) SetQuietMode(quiet bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.quietMode = quiet
}

// SetSuppressDetails enables or disables detailed message suppression for mass operations
func (p *ProgressReporter) SetSuppressDetails(suppress bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hideRequests = suppress
}

// UpdateProgress updates progress with a custom message without creating new lines
func (p *ProgressReporter) UpdateProgress(current, total int, message string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = current
	p.total = total

	if p.hideRequests {
		// For mass operations, show only a dynamic progress line
		p.updateProgressLine(message)
	} else {
		// For detailed operations, show full progress
		p.update()
	}
}

// updateProgressLine shows a single-line progress indicator
func (p *ProgressReporter) updateProgressLine(customMessage string) {
	p.clearLine()

	// Calculate progress percentage
	var percent float64
	if p.total > 0 {
		percent = float64(p.current) * 100 / float64(p.total)
	}

	// Calculate elapsed time
	elapsed := time.Since(p.startTime)
	elapsedStr := formatDuration(elapsed)

	// Calculate ETA
	var etaStr string
	if p.current > 0 && p.total > 0 && p.current < p.total {
		eta := time.Duration(float64(elapsed) * float64(p.total-p.current) / float64(p.current))
		etaStr = formatDuration(eta)
	} else if p.current >= p.total {
		etaStr = "complete"
	} else {
		etaStr = "calculating..."
	}

	// Create a compact progress bar
	const barWidth = 20
	completedWidth := int(float64(barWidth) * float64(p.current) / float64(p.total))
	if completedWidth > barWidth {
		completedWidth = barWidth
	}

	bar := "["
	for i := 0; i < barWidth; i++ {
		if i < completedWidth {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	bar += "]"

	// Show spinner for active operations
	spinner := p.spinnerStates[p.spinnerIndex]
	p.spinnerIndex = (p.spinnerIndex + 1) % len(p.spinnerStates)

	// Format the progress line
	if customMessage != "" {
		fmt.Fprintf(p.writer, "%s %s %s%s%s %s%.1f%%%s [%s%d/%d%s] %s%s%s | %s%s%s\r",
			spinner,
			p.testName,
			ColorGreen, bar, ColorReset,
			ColorYellow, percent, ColorReset,
			ColorCyan, p.current, p.total, ColorReset,
			ColorGray, elapsedStr, ColorReset,
			ColorBlue, customMessage, ColorReset,
		)
	} else {
		fmt.Fprintf(p.writer, "%s %s %s%s%s %s%.1f%%%s [%s%d/%d%s] %s%s%s | ETA: %s%s%s\r",
			spinner,
			p.testName,
			ColorGreen, bar, ColorReset,
			ColorYellow, percent, ColorReset,
			ColorCyan, p.current, p.total, ColorReset,
			ColorGray, elapsedStr, ColorReset,
			ColorGray, etaStr, ColorReset,
		)
	}
}

// FinishProgress completes the progress and shows final status
func (p *ProgressReporter) FinishProgress(successCount, errorCount int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.clearLine()
	elapsed := time.Since(p.startTime)

	if errorCount == 0 {
		fmt.Fprintf(p.writer, "%s✓ %s%s%s: %s%d%s requests completed successfully in %s%s%s\n",
			ColorGreen,
			ColorCyan, p.testName, ColorReset,
			ColorGreen, successCount, ColorReset,
			ColorGray, formatDuration(elapsed), ColorReset,
		)
	} else {
		fmt.Fprintf(p.writer, "%s⚠ %s%s%s: %s%d%s successful, %s%d%s errors in %s%s%s\n",
			ColorYellow,
			ColorCyan, p.testName, ColorReset,
			ColorGreen, successCount, ColorReset,
			ColorRed, errorCount, ColorReset,
			ColorGray, formatDuration(elapsed), ColorReset,
		)
	}
}
