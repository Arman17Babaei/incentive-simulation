package utils

import (
	"go-incentive-simulation/config"
	"go-incentive-simulation/model/parts/types"
	"math"
)

func CheckForgiveness(edgeData types.EdgeAttrs, firstNodeId int, secondNodeId int, graph *types.Graph, request types.Request) (int, bool) {
	//passedTime := (request.TimeStep - edgeData.Last) / constants.GetRequestsPerSecond()
	passedTime := request.Epoch - edgeData.EpokeLastForgiven
	if passedTime > 0 {
		refreshRate := config.GetRefreshRate()
		if config.IsAdjustableThreshold() {
			refreshRate = int(math.Ceil(float64(edgeData.Threshold / 2)))
		}
		removedDeptAmount := passedTime * refreshRate
		newEdgeData := edgeData
		newEdgeData.A2B -= removedDeptAmount
		if newEdgeData.A2B < 0 {
			newEdgeData.A2B = 0
		}
		//newEdgeData.Last = request.LastEpoch
		newEdgeData.EpokeLastForgiven = request.Epoch
		graph.SetEdgeData(firstNodeId, secondNodeId, newEdgeData)
		return newEdgeData.A2B, true
	}
	return edgeData.A2B, false
}
