// Package cloud provides Firebase Firestore integration for WiFi Sentinel.
// This file implements a thread-safe sample buffer for offline resilience.
// When the cloud is unreachable, samples accumulate in the buffer and are
// flushed automatically when connectivity returns.
package cloud

import (
	"log"
	"sync"
	"time"
)

const (
	// MaxBufferSize is the maximum number of samples to hold in memory.
	// Beyond this, the oldest samples are dropped.
	MaxBufferSize = 1000

	// FlushInterval is how often the buffer attempts to push to Firestore.
	FlushInterval = 30 * time.Second

	// FlushBatchSize is the max number of samples to push per flush.
	FlushBatchSize = 50
)

// SampleBuffer is a thread-safe ring buffer of pending network samples.
// It accumulates samples when the cloud is unreachable and flushes them
// when connectivity returns.
type SampleBuffer struct {
	mu      sync.Mutex
	samples []NetworkSampleData
	dropped uint64 // total number of dropped samples (for logging)
}

// NewSampleBuffer creates a new empty sample buffer.
func NewSampleBuffer() *SampleBuffer {
	return &SampleBuffer{
		samples: make([]NetworkSampleData, 0, 128),
	}
}

// Add enqueues a sample for cloud upload. If the buffer is full,
// the oldest sample is dropped.
func (b *SampleBuffer) Add(sample NetworkSampleData) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.samples) >= MaxBufferSize {
		// Drop the oldest sample
		b.samples = b.samples[1:]
		b.dropped++
		if b.dropped%100 == 0 {
			log.Printf("[cloud] buffer overflow — %d samples dropped total", b.dropped)
		}
	}
	b.samples = append(b.samples, sample)
}

// Len returns the current number of buffered samples.
func (b *SampleBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.samples)
}

// Drain removes and returns up to `max` samples from the front of the buffer.
func (b *SampleBuffer) Drain(max int) []NetworkSampleData {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.samples) == 0 {
		return nil
	}

	n := max
	if n > len(b.samples) {
		n = len(b.samples)
	}

	batch := make([]NetworkSampleData, n)
	copy(batch, b.samples[:n])
	b.samples = b.samples[n:]
	return batch
}

// Flush attempts to push all buffered samples to Firestore via the given client.
// Samples are pushed in batches. If a batch fails, remaining samples stay in the buffer.
func (b *SampleBuffer) Flush(client *FirebaseClient, sessionID string) {
	if !client.IsAuthenticated() || sessionID == "" {
		return
	}

	for {
		batch := b.Drain(FlushBatchSize)
		if batch == nil || len(batch) == 0 {
			return
		}

		err := client.PushSamples(sessionID, batch)
		if err != nil {
			log.Printf("[cloud] flush failed (%d samples), re-buffering: %v", len(batch), err)
			// Put samples back at the front
			b.mu.Lock()
			b.samples = append(batch, b.samples...)
			// Trim if over capacity
			if len(b.samples) > MaxBufferSize {
				excess := len(b.samples) - MaxBufferSize
				b.samples = b.samples[excess:]
				b.dropped += uint64(excess)
			}
			b.mu.Unlock()
			return
		}

		log.Printf("[cloud] flushed %d samples to Firestore", len(batch))
	}
}

// SpeedTestBuffer is a simple queue for pending speed test uploads.
type SpeedTestBuffer struct {
	mu    sync.Mutex
	tests []SpeedTestData
}

// NewSpeedTestBuffer creates a new empty speed test buffer.
func NewSpeedTestBuffer() *SpeedTestBuffer {
	return &SpeedTestBuffer{
		tests: make([]SpeedTestData, 0, 8),
	}
}

// Add enqueues a speed test result for cloud upload.
func (b *SpeedTestBuffer) Add(data SpeedTestData) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tests = append(b.tests, data)
}

// Flush pushes all buffered speed tests to Firestore.
func (b *SpeedTestBuffer) Flush(client *FirebaseClient, sessionID string) {
	if !client.IsAuthenticated() || sessionID == "" {
		return
	}

	b.mu.Lock()
	tests := b.tests
	b.tests = make([]SpeedTestData, 0, 8)
	b.mu.Unlock()

	for _, t := range tests {
		if err := client.PushSpeedTest(sessionID, t); err != nil {
			log.Printf("[cloud] speed test push failed: %v", err)
			// Re-buffer failed tests
			b.mu.Lock()
			b.tests = append([]SpeedTestData{t}, b.tests...)
			b.mu.Unlock()
			return
		}
		log.Printf("[cloud] pushed speed test to Firestore")
	}
}
