package resampler

import (
	"math"

	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

func calculateTriangleArea(paX, paY, pbX, pbY, pcX, pcY schema.Float) float64 {
	area := ((paX-pcX)*(pbY-paY) - (paX-pbX)*(pcY-paY)) * 0.5
	return math.Abs(float64(area))
}

func calculateAverageDataPoint(points []schema.Float, xStart int64) (avgX schema.Float, avgY schema.Float) {
	flag := 0
	for _, point := range points {
		avgX += schema.Float(xStart)
		avgY += point
		xStart++
		if math.IsNaN(float64(point)) {
			flag = 1
		}
	}

	l := schema.Float(len(points))

	avgX /= l
	avgY /= l

	if flag == 1 {
		return avgX, schema.NaN
	} else {
		return avgX, avgY
	}
}
