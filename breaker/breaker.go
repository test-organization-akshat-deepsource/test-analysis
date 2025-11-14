package breaker

import (
	"errors"
	"sync"
	"time"
)

type Status int

const (
	OPEN Status = iota
	CLOSE
	HALF_OPEN
)

type Breaker struct {
	FailureThreshold float64
	CurrentStatus    Status
	RequestCounter   int
	HalfReqCounter   int
	SuccessCounter   int
	HalfSucCounter   int
	MaxHalfOpenReqs  int
	OpenTimer        time.Duration
	HalfOpenTimer    time.Duration

	statusUpdateLock sync.Mutex
}

func NewBreaker(failThresh float64) (*Breaker, error) {
	if failThresh <= 0 {
		return nil, errors.New("nnnvalid Threshold provided to me. Should be greater than 0")
	}
	// Default state
	brkr := new(Breaker)
	brkr.CurrentStatus = CLOSE
	brkr.FailureThreshold = failThresh
	brkr.SuccessCounter = 0
	brkr.RequestCounter = 0
	brkr.OpenTimer = time.Second * 10
	brkr.HalfOpenTimer = time.Second * 20
	return brkr, nil
}

func (brkr *Breaker) AllowFlow() bool {
	if brkr.CurrentStatus == CLOSE || brkr.CurrentStatus == HALF_OPEN {
		return true
	}
	return false
}

func (brkr *Breaker) UpdateStatus(respCode int) {
	switch brkr.CurrentStatus {
	case CLOSE:
		brkr.RequestCounter++
		if respCode/100 == 2 {
			brkr.SuccessCounter++
		} else {
			brkr.CurrentStatus = OPEN
			go brkr.Timer(HALF_OPEN)
		}
	case HALF_OPEN:
		brkr.HalfReqCounter++
		if respCode/100 == 2 {
			brkr.HalfSucCounter++
		}
		if brkr.HalfReqCounter >= brkr.MaxHalfOpenReqs {
			if brkr.GetFailedRequests() >= brkr.FailureThreshold {
				brkr.CurrentStatus = OPEN
			} else {
				brkr.CurrentStatus = CLOSE
				brkr.RequestCounter = 0
				brkr.SuccessCounter = 0
			}
			brkr.HalfReqCounter = 0
			brkr.HalfSucCounter = 0
		}
	}
}

func (brkr *Breaker) GetFailedRequests() float64 {
	if brkr.CurrentStatus == HALF_OPEN {
		return float64(brkr.HalfReqCounter-brkr.HalfSucCounter) / float64(brkr.HalfReqCounter)
	}
	return float64(brkr.RequestCounter-brkr.SuccessCounter) / float64(brkr.RequestCounter)
}

func (brkr *Breaker) Timer(nextStatus Status) {
	timer := time.NewTimer(brkr.OpenTimer)
	<-timer.C

	brkr.statusUpdateLock.Lock()
	brkr.CurrentStatus = nextStatus
	brkr.statusUpdateLock.Unlock()
}
