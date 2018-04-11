package venom

import (
	"github.com/vbauerster/mpb"
	"github.com/vbauerster/mpb/decor"
	pb "gopkg.in/cheggaaa/pb.v1"

	log "github.com/sirupsen/logrus"
)

func (v *Venom) initBars() *mpb.Progress {

	pool := mpb.New(mpb.Output(v.LogOutput))

	for _, ts := range v.testsuites {
		nSteps := 0
		for _, tc := range ts.TestCases {
			nSteps += len(tc.TestSteps)
			if len(tc.Skipped) >= 1 {
				ts.Skipped += len(tc.Skipped)
			}
		}
		bar := pool.AddBar(int64(nSteps),
			mpb.PrependDecorators(
				decor.StaticName(ts.Name, 0, 0),
				// DSyncSpace is shortcut for DwidthSync|DextraSpace
				// means sync the width of respective decorator's column
				// and prepend one extra space.
				decor.Percentage(3, decor.DSyncSpace),
			),
			mpb.AppendDecorators(
				//decor.ETA(2, 0),
				decor.Elapsed(2, 0),
			),
		)
		v.outputProgressBar[ts.Name] = bar
	}
	// //var pool *pb.Pool
	// var pbbars []*pb.ProgressBar
	// // sort bars pool
	// var keys []string
	// for k := range v.outputProgressBar {
	// 	keys = append(keys, k)
	// }
	// sort.Strings(keys)
	// for _, k := range keys {
	// 	pbbars = append(pbbars, v.outputProgressBar[k])
	// }
	// var errs error

	// pool, errs = pb.StartPool(pbbars...)
	// pool.RefreshRate = (30 * time.Millisecond)
	// if errs != nil {
	// 	log.Errorf("Error while prepare details bars: %s", errs)
	// 	pool = nil
	// }
	// pool.Output = v.LogOutput
	return pool
}

func endBars(detailsLevel string, pool *pb.Pool) {
	if detailsLevel != DetailsLow && pool != nil {
		if err := pool.Stop(); err != nil {
			log.Errorf("Error while closing pool progress bar: %s", err)
		}
	}
}
