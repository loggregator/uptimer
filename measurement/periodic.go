package measurement

import (
	"fmt"
	"log"
	"time"

	"github.com/benbjohnson/clock"
)

type periodic struct {
	logger             *log.Logger
	clock              clock.Clock
	freq               time.Duration
	baseMeasurement    BaseMeasurement
	shouldRetryFunc    ShouldRetryFunc
	allowedFailures    int
	measureImmediately bool

	resultSet ResultSet
	stopChan  chan int
}

func (p *periodic) Name() string {
	return p.baseMeasurement.Name()
}

func (p *periodic) Start() {
	ticker := p.clock.Ticker(p.freq)
	go func() {
		if p.measureImmediately {
			p.performMeasurement()
		}
		for {
			select {
			case <-ticker.C:
				p.performMeasurement()
			case <-p.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
}

func (p *periodic) performMeasurement() {
	if msg, stdOut, stdErr, ok := p.performWithSingleRetry(); !ok {
		p.logFailure(msg, stdOut, stdErr)
		p.resultSet.RecordFailure()
		return
	}

	p.resultSet.RecordSuccess()
}

func (p *periodic) performWithSingleRetry() (string, string, string, bool) {
	msg, stdOut, stdErr, ok := p.baseMeasurement.PerformMeasurement()
	if !ok && p.shouldRetryFunc(stdOut, stdErr) {
		return p.baseMeasurement.PerformMeasurement()
	}

	return msg, stdOut, stdErr, ok
}

func (p *periodic) logFailure(msg, stdOut, stdErr string) {
	var lfMsg string
	if sslf, lf := p.resultSet.SuccessesSinceLastFailure(); sslf > 0 {
		lfMsg = fmt.Sprintf(" (%d successes since last failure at %s)", sslf, lf.Format("2006/01/02 15:04:05"))
	}

	var stdOutMsg string
	if stdOut != "" {
		stdOutMsg = fmt.Sprintf("\nstdout:\n%s\n", stdOut)
	}

	var stdErrMsg string
	if stdErr != "" {
		stdErrMsg = fmt.Sprintf("\nstderr:\n%s\n", stdErr)
	}

	p.logger.Printf(
		"\x1b[31mFAILURE (%s): %s%s\x1b[0m\n%s%s\n",
		p.baseMeasurement.Name(),
		msg,
		lfMsg,
		stdOutMsg,
		stdErrMsg,
	)
}

func (p *periodic) Results() ResultSet {
	return p.resultSet
}

func (p *periodic) Stop() {
	p.stopChan <- 0
}

func (p *periodic) Failed() bool {
	return p.resultSet.Failed() > p.allowedFailures
}

func (p *periodic) Summary() string {
	msg := "SUCCESS (%s): %d failed attempts to %s did not exceed the threshold of %d allowed failures (Total attempts: %d)"
	if p.Failed() {
		msg = "FAILED (%s): %d failed attempts to %s exceeded the threshold of %d allowed failures (Total attempts: %d)"
	}

	return fmt.Sprintf(
		msg,
		p.baseMeasurement.Name(),
		p.resultSet.Failed(),
		p.baseMeasurement.SummaryPhrase(),
		p.allowedFailures,
		p.resultSet.Total(),
	)
}
