package utils

import (
	"go-incentive-simulation/config"
	"go-incentive-simulation/model/general"
	"go-incentive-simulation/model/parts/types"
	"sort"
)

func PrecomputeRespNodes(nodesId []int) [][4]int {
	numPossibleChunks := config.GetRangeAddress()
	result := make([][4]int, numPossibleChunks)
	numNodesSearch := config.GetBits()

	for chunkId := 0; chunkId < numPossibleChunks; chunkId++ {

		closestNodes := general.BinarySearchClosest(nodesId, chunkId, numNodesSearch)
		distances := make([]int, len(closestNodes))

		for j, nodeId := range closestNodes {
			distances[j] = nodeId ^ chunkId
		}

		sort.Slice(distances, func(i, j int) bool { return distances[i] < distances[j] })

		for k := 0; k < 4; k++ {
			result[chunkId][k] = distances[k] ^ chunkId // this results in the nodeId again
		}
	}

	return result
}

func SortedKeys(m map[int]*types.Node) []int {
	keys := make([]int, len(m))
	i := 0
	for k := range m {
		keys[i] = k
		i++
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func CreateGraphNetwork(net *types.Network) (*types.Graph, error) {
	//fmt.Println("Creating graph network...")
	sortedNodeIds := SortedKeys(net.NodesMap)
	numNodes := len(net.NodesMap)
	Edges := make(map[int]map[int]*types.Edge)
	respNodes := make([][4]int, config.GetRangeAddress())
	if config.IsPrecomputeRespNodes() {
		respNodes = PrecomputeRespNodes(sortedNodeIds)
	}

	graph := &types.Graph{
		Network:   net,
		Nodes:     make([]*types.Node, 0, numNodes),
		Edges:     Edges,
		NodeIds:   sortedNodeIds,
		RespNodes: respNodes,
	}

	for _, nodeId := range sortedNodeIds {
		graph.Edges[nodeId] = make(map[int]*types.Edge)

		node := net.NodesMap[nodeId]
		err1 := graph.AddNode(node)
		if err1 != nil {
			return nil, err1
		}

		nodeAdj := node.AdjIds
		for _, adjItems := range nodeAdj {
			for _, item := range adjItems {
				threshold := general.BitLength(nodeId ^ item)
				epoke := config.GetEpoch()
				attrs := types.EdgeAttrs{A2B: 0, Last: 0, EpokeLastForgiven: epoke, Threshold: threshold}
				err := graph.AddEdge(node.Id, item, attrs)
				if err != nil {
					return nil, err
				}
				// graph.SetEdgeAttributes()
			}
		}
	}

	//fmt.Println("Graph network is created.")
	return graph, nil
}

func isThresholdFailed(firstNodeId int, secondNodeId int, chunkId int, graph *types.Graph, request types.Request) bool {
	if config.GetThresholdEnabled() {
		edgeDataFirst := graph.GetEdgeData(firstNodeId, secondNodeId)
		p2pFirst := edgeDataFirst.A2B
		edgeDataSecond := graph.GetEdgeData(secondNodeId, firstNodeId)
		p2pSecond := edgeDataSecond.A2B

		threshold := config.GetThreshold()
		if config.IsAdjustableThreshold() {
			threshold = edgeDataFirst.Threshold
		}

		peerPriceChunk := PeerPriceChunk(secondNodeId, chunkId)
		price := p2pFirst - p2pSecond + peerPriceChunk
		//fmt.Printf("price: %d = p2pFirst: %d - p2pSecond: %d + PeerPriceChunk: %d \n", price, p2pFirst, p2pSecond, peerPriceChunk)

		if price > threshold {
			if config.IsForgivenessDuringRouting() && config.IsForgivenessEnabled() {
				newP2pFirst, forgivenFirst := CheckForgiveness(edgeDataFirst, firstNodeId, secondNodeId, graph, request)
				//_, _ = CheckForgiveness(edgeDataSecond, secondNodeId, firstNodeId, graph, request)
				if forgivenFirst {
					price = newP2pFirst - p2pSecond + peerPriceChunk
				}
			}
		}
		return price > threshold
	}
	return false
}

func getNext(firstNodeId int, chunkId int, graph *types.Graph, mainOriginatorId int, prevNodePaid bool, rerouteStruct types.RerouteStruct, request types.Request) (int, []types.Threshold, bool, bool, types.Payment, bool) {
	var nextNodeId = 0
	var payNextId = 0
	var thresholdList []types.Threshold
	var thresholdFailed bool
	var accessFailed bool
	var payment types.Payment
	lastDistance := firstNodeId ^ chunkId
	//fmt.Printf("\n last distance is : %d, chunk is: %d, first is: %d", lastDistance, chunkId, firstNodeId)
	//fmt.Printf("\n which bucket: %d \n", 16-BitLength(chunkId^firstNodeId))

	currDist := lastDistance
	payDist := lastDistance

	//var lockedEdges []int
	//var unlockedEdges []int

	//firstNode := graph.NodesMap[firstNodeId]
	bin := config.GetBits() - general.BitLength(firstNodeId^chunkId)
	firstNodeAdjIds := graph.GetNodeAdj(firstNodeId)

	for _, nodeId := range firstNodeAdjIds[bin] {
		dist := nodeId ^ chunkId
		if general.BitLength(dist) >= general.BitLength(lastDistance) {
			continue
		}
		if dist >= currDist {
			continue
		}
		// This means the node is now actively trying to communicate with the other node
		if config.GetEdgeLock() {
			graph.LockEdge(firstNodeId, nodeId)
		}
		if !isThresholdFailed(firstNodeId, nodeId, chunkId, graph, request) {
			thresholdFailed = false
			if config.IsRetryWithAnotherPeer() {
				if reroute := rerouteStruct.GetRerouteMap(mainOriginatorId).Reroute; reroute != nil {
					if general.Contains(reroute, nodeId) {
						if config.GetEdgeLock() {
							graph.UnlockEdge(firstNodeId, nodeId)
						}
						continue // skips node that's been part of a failed route before
					}
				}
			}

			if config.GetEdgeLock() {
				if nextNodeId != 0 {
					graph.UnlockEdge(firstNodeId, nextNodeId)
				}
				if payNextId != 0 {
					graph.UnlockEdge(firstNodeId, payNextId)
					payNextId = 0 // IMPORTANT!
				}
			}
			currDist = dist
			nextNodeId = nodeId

		} else {
			thresholdFailed = true
			if config.GetPaymentEnabled() {
				if dist < payDist && nextNodeId == 0 {
					if config.GetEdgeLock() && payNextId != 0 {
						graph.UnlockEdge(firstNodeId, payNextId)
					}
					payDist = dist
					payNextId = nodeId
				} else {
					if config.GetEdgeLock() {
						graph.UnlockEdge(firstNodeId, nodeId)
					}
				}
			} else {
				if config.GetEdgeLock() {
					graph.UnlockEdge(firstNodeId, nodeId)
				}
			}
		}
	}

	if nextNodeId != 0 {
		thresholdFailed = false
		accessFailed = false
	} else if !thresholdFailed {
		accessFailed = true
		nextNodeId = -2 // Access Failed
	} else {
		nextNodeId = -1 // Threshold Failed
	}

	if config.GetPaymentEnabled() && payNextId != 0 {
		accessFailed = false

		if config.IsOnlyOriginatorPays() {
			if firstNodeId == mainOriginatorId {
				payment.IsOriginator = true
				payment.FirstNodeId = firstNodeId
				payment.PayNextId = payNextId
				payment.ChunkId = chunkId
				nextNodeId = payNextId
			}

		} else if config.IsPayIfOrigPays() {
			if prevNodePaid {
				thresholdFailed = false
				if firstNodeId == mainOriginatorId {
					payment.IsOriginator = true
				} else {
					payment.IsOriginator = false
				}
				payment.FirstNodeId = firstNodeId
				payment.PayNextId = payNextId
				payment.ChunkId = chunkId
				nextNodeId = payNextId

			} else if firstNodeId == mainOriginatorId {
				payment.IsOriginator = true
				payment.FirstNodeId = firstNodeId
				payment.PayNextId = payNextId
				payment.ChunkId = chunkId
				nextNodeId = payNextId

			} else {
				thresholdFailed = true
				nextNodeId = -1
				payNextId = 0
			}

		} else {
			thresholdFailed = false
			if firstNodeId == mainOriginatorId {
				payment.IsOriginator = true
			} else {
				payment.IsOriginator = false
			}
			payment.FirstNodeId = firstNodeId
			payment.PayNextId = payNextId
			payment.ChunkId = chunkId
			nextNodeId = payNextId
		}
	}

	//// unlocks all nodes except the nextNodeId lock
	//if constants.GetEdgeLock() {
	//	for _, nodeId := range lockedEdges {
	//		if nodeId != nextNodeId {
	//			graph.UnlockEdge(firstNodeId, nodeId)
	//		}
	//	}
	//}

	if config.GetPaymentEnabled() {
	out:
		for i, item := range thresholdList {
			for _, nodeId := range item {
				if nodeId == payNextId {
					if config.IsPayIfOrigPays() {
						if firstNodeId == mainOriginatorId {
							thresholdList = append(thresholdList[:i], thresholdList[i+1:]...)
						}
					} else {
						thresholdList = append(thresholdList[:i], thresholdList[i+1:]...)
					}
					break out
				}
			}
		}
	}

	if payment != (types.Payment{}) {
		prevNodePaid = true
	} else {
		prevNodePaid = false
	}
	// RASMUS: nil reference error
	if nextNodeId != 0 {
		// fmt.Println("Next node is: ", nextNodeId)
	}
	//nextNodeId, thresholdList, _, accessFailed, payment, prevNodePaid
	return nextNodeId, thresholdList, thresholdFailed, accessFailed, payment, prevNodePaid
}

// ConsumeTask cacheDict is map of nodes containing an array of maps with key as a chunkAddr and a popularity counter
func ConsumeTask(request types.Request, graph *types.Graph, rerouteStruct types.RerouteStruct, cacheStruct types.CacheStruct) (bool, types.Route, [][]types.Threshold, bool, []types.Payment) {
	var thresholdFailedList [][]types.Threshold
	var paymentList []types.Payment
	originatorId := request.OriginatorId
	chunkId := request.ChunkId
	respNodes := request.RespNodes
	mainOriginatorId := originatorId
	found := false
	foundByCaching := false
	route := types.Route{mainOriginatorId}
	//var resultInt int
	var nextNodeId int
	var thresholdList []types.Threshold
	// thresholdFailed := false
	var accessFailed bool
	var payment types.Payment
	var prevNodePaid bool

	if config.IsPayIfOrigPays() {
		prevNodePaid = true
	}
	if general.ArrContains(respNodes, mainOriginatorId) {
		// originator has the chunk
		found = true
	} else {
	out:
		for !general.ArrContains(respNodes, originatorId) {

			//fmt.Printf("\n orig: %d, chunk_id: %d", mainOriginatorId, chunkId)
			//nextNodeId, thresholdList, _, accessFailed, payment, prevNodePaid = getNext(originatorId, chunkId, graph, mainOriginatorId, prevNodePaid, rerouteMap)

			nextNodeId, thresholdList, _, accessFailed, payment, prevNodePaid = getNext(originatorId, chunkId, graph, mainOriginatorId, prevNodePaid, rerouteStruct, request)

			//if nextNodeId == -2 {
			//	// Access Failed
			//	fmt.Println("Access Failed")
			//}

			if payment != (types.Payment{}) {
				paymentList = append(paymentList, payment)
			}
			if len(thresholdList) > 0 {
				thresholdFailedList = append(thresholdFailedList, thresholdList)
			}
			// RASMUS: Nil reference error
			if nextNodeId != 0 {
				route = append(route, nextNodeId)
			}
			// if not isinstance(next_node, int), originale versjonen
			if !(nextNodeId <= -1) {
				if general.ArrContains(respNodes, nextNodeId) {
					//fmt.Println("is not in cache")
					found = true
					break out
				}
				if config.IsCacheEnabled() {
					//if ok := cacheStruct.Contains(nextNodeId, chunkId); ok {
					//	found = true
					//	foundByCaching = true
					//	break out
					//}
					node := graph.GetNode(nextNodeId)
					node.Mutex.Lock()
					if _, ok := node.CacheMap[chunkId]; ok {
						//fmt.Println("is in cache")
						found = true
						foundByCaching = true
						node.Mutex.Unlock()
						break out
					}
					node.Mutex.Unlock()
				}
				// NOTE !
				originatorId = nextNodeId
			} else {
				break out
			}
		}
	}

	route = append(route, chunkId)

	if config.IsForwarderPayForceOriginatorToPay() {
		//if nextNodeId != -2 {
		if !general.Contains(route, -2) {
			// NOT accessFailed
			if len(paymentList) > 0 {
				firstPayment := paymentList[0]
				if !firstPayment.IsOriginator {
					for i := range route[:len(route)-1] {
						p := types.Payment{FirstNodeId: route[i], PayNextId: route[i+1], ChunkId: route[len(route)-1]}

						for _, tmp := range paymentList {
							if p.PayNextId == tmp.PayNextId && p.FirstNodeId == tmp.FirstNodeId && p.ChunkId == tmp.ChunkId {
								break
							}
						}
						// payment is now not in paymentList
						if i == 0 {
							p.IsOriginator = true
						}
						if i != len(route)-2 {
							if i != len(route)-3 {
								paymentList = append(paymentList[:i+1], paymentList[i:]...)
							}
							paymentList[i] = p
						} else {
							continue
						}

					}
				} else {
					for i := range route[1 : len(route)-1] {
						p := types.Payment{FirstNodeId: route[i], PayNextId: route[i+1], ChunkId: route[len(route)-1]}
						for _, tmp := range paymentList {
							if p.PayNextId == tmp.PayNextId && p.FirstNodeId == tmp.FirstNodeId && p.ChunkId == tmp.ChunkId {
								break
							}
						}
						// payment is now not in paymentList
						if i != len(route)-2 {
							if i != len(route)-3 {
								paymentList = append(paymentList[:i+1], paymentList[i:]...)
							}
							paymentList[i] = p
						} else {
							continue
						}
					}
				}
			}
		} else {
			paymentList = []types.Payment{}
		}

	}
	if foundByCaching {
		// route = append(route, "C") // TYPE MISMATCH
		route = append(route, -3) // TODO: midlertidig fix?
	}
	return found, route, thresholdFailedList, accessFailed, paymentList
}

func getProximityChunk(firstNodeId int, chunkId int) int {
	retVal := config.GetBits() - general.BitLength(firstNodeId^chunkId)
	if retVal <= config.GetMaxProximityOrder() {
		return retVal
	} else {
		return config.GetMaxProximityOrder()
	}
}

func PeerPriceChunk(firstNodeId int, chunkId int) int {
	val := (config.GetMaxProximityOrder() - getProximityChunk(firstNodeId, chunkId) + 1) * config.GetPrice()
	return val
}

func CreateDownloadersList(g *types.Graph) []int {
	//fmt.Println("Creating downloaders list...")

	downloadersList := general.Choice(g.NodeIds, config.GetOriginators())

	//fmt.Println("Downloaders list create...!")
	return downloadersList
}

func CreateNodesList(g *types.Graph) []int {
	//fmt.Println("Creating nodes list...")
	nodesValue := g.NodeIds
	//fmt.Println("NodesMap list create...!")
	return nodesValue
}

// TODO: Not used in original
//func getBin(src int, dest int, index int) int {
//	distance := src ^ dest
//	result := index
//	for distance > 0 {
//		distance >>= 1
//		result -= 1
//	}
//	return result
//}

// TODO: Not used in original
//func whichPowerTwo(rangeAddress int) int {
//	return BitLength(rangeAddress) - 1
//}

// TODO: Not used in original
//func MakeFiles() []int {
//	fmt.Println("Making files...")
//	var filesList []int
//
//	for i := 0; i <= ct.constants.GetOriginators(); i++ {
//		// chunksList := choice(ct.constants.GetChunks(), ct.constants.GetRangeAddress())
//		// filesList = append(chunksList)
//		fmt.Println(i)
//	}
//	// Gets all constants
//	consts := ct.constants
//
//	for i := 0; i <= consts.GetOriginators(); i++ {
//		chunksList := rand.Perm(consts.GetChunks())
//		filesList = append(chunksList)
//	}
//	fmt.Println("Files made!")
//	return filesList
//}

// TODO: Not used in original
//func (net *Network) PushSync(fileName string, files []string) {
//	fmt.Println("Pushing sync...")
//	if net == nil {
//		fmt.Println("Network is nil!")
//		return
//	}
//	nodes := net.nodes
//	for i := range nodes {
//		fmt.Println(nodes[i].id)
//	}
//
//	fmt.Println("Pushing sync finished...")
//}
