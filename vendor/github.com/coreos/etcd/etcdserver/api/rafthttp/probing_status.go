// Copyright 2015 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rafthttp

import (
	"time"

	"github.com/xiang90/probing"
	"go.uber.org/zap"
)

var (
	// proberInterval must be shorter than read timeout.
	// Or the connection will time-out.
	proberInterval           = ConnReadTimeout - time.Second
	statusMonitoringInterval = 30 * time.Second
	statusErrorInterval      = 5 * time.Second
)

func addPeerToProber(lg *zap.Logger, p probing.Prober, id string, us []string) {
	hus := make([]string, len(us))
	for i := range us {
		hus[i] = us[i] + ProbingPrefix
	}

	p.AddHTTP(id, proberInterval, hus)

	s, err := p.Status(id)
	if err != nil {
		if lg != nil {
			lg.Warn("failed to add peer into prober", zap.String("remote-peer-id", id))
		} else {
			plog.Errorf("failed to add peer %s into prober", id)
		}
		return
	}

	go monitorProbingStatus(lg, s, id)
}

func monitorProbingStatus(lg *zap.Logger, s probing.Status, id string) {
	// set the first interval short to log error early.
	interval := statusErrorInterval
	for {
		select {
		case <-time.After(interval):
			if !s.Health() {
				if lg != nil {
					lg.Warn(
						"prober detected unhealthy status",
						zap.String("remote-peer-id", id),
						zap.Duration("rtt", s.SRTT()),
						zap.Error(s.Err()),
					)
				} else {
					plog.Warningf("health check for peer %s could not connect: %v", id, s.Err())
				}
				interval = statusErrorInterval
			} else {
				interval = statusMonitoringInterval
			}
			if s.ClockDiff() > time.Second {
				if lg != nil {
					lg.Warn(
						"prober found high clock drift",
						zap.String("remote-peer-id", id),
						zap.Duration("clock-drift", s.SRTT()),
						zap.Duration("rtt", s.ClockDiff()),
						zap.Error(s.Err()),
					)
				} else {
					plog.Warningf("the clock difference against peer %s is too high [%v > %v]", id, s.ClockDiff(), time.Second)
				}
			}
			rttSec.WithLabelValues(id).Observe(s.SRTT().Seconds())

		case <-s.StopNotify():
			return
		}
	}
}
