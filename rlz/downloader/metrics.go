// Copyright 2019 The go-relianz Authors
// This file is part of the go-relianz library.
//
// The go-relianz library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-relianz library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-relianz library. If not, see <http://www.gnu.org/licenses/>.

// Contains the metrics collected by the downloader.

package downloader

import (
	"github.com/relianz2019/relianz/metrics"
)

var (
	headerInMeter      = metrics.NewRegisteredMeter("rlz/downloader/headers/in", nil)
	headerReqTimer     = metrics.NewRegisteredTimer("rlz/downloader/headers/req", nil)
	headerDropMeter    = metrics.NewRegisteredMeter("rlz/downloader/headers/drop", nil)
	headerTimeoutMeter = metrics.NewRegisteredMeter("rlz/downloader/headers/timeout", nil)

	bodyInMeter      = metrics.NewRegisteredMeter("rlz/downloader/bodies/in", nil)
	bodyReqTimer     = metrics.NewRegisteredTimer("rlz/downloader/bodies/req", nil)
	bodyDropMeter    = metrics.NewRegisteredMeter("rlz/downloader/bodies/drop", nil)
	bodyTimeoutMeter = metrics.NewRegisteredMeter("rlz/downloader/bodies/timeout", nil)

	receiptInMeter      = metrics.NewRegisteredMeter("rlz/downloader/receipts/in", nil)
	receiptReqTimer     = metrics.NewRegisteredTimer("rlz/downloader/receipts/req", nil)
	receiptDropMeter    = metrics.NewRegisteredMeter("rlz/downloader/receipts/drop", nil)
	receiptTimeoutMeter = metrics.NewRegisteredMeter("rlz/downloader/receipts/timeout", nil)

	stateInMeter   = metrics.NewRegisteredMeter("rlz/downloader/states/in", nil)
	stateDropMeter = metrics.NewRegisteredMeter("rlz/downloader/states/drop", nil)
)
