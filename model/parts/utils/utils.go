package utils

import (
	"fmt"
	. "go-incentive-simulation/model/constants"
	. "go-incentive-simulation/model/general"
	. "go-incentive-simulation/model/parts/types"
	"sort"
)

func SortedKeys(m map[int]*Node) []int {
	keys := make([]int, len(m))
	i := 0
	for k := range m {
		keys[i] = k
		i++
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func CreateGraphNetwork(net *Network) (*Graph, error) {
	fmt.Println("Creating graph network...")
	sortedNodeIds := SortedKeys(net.Nodes)
	numNodes := len(net.Nodes)
	graph := &Graph{
		Network: net,
		Nodes:   make([]*Node, 0, numNodes),
		Edges:   make(map[int][]Edge, numNodes),
		NodeIds: sortedNodeIds,
	}

	for _, nodeId := range sortedNodeIds {
		node := net.Nodes[nodeId]
		err1 := graph.AddNode(net.Nodes[nodeId])
		if err1 != nil {
			return nil, err1
		}

		nodeAdj := node.Adj
		for _, adjItems := range nodeAdj {
			for _, item := range adjItems {
				threshold := BitLength(nodeId ^ item.Id)
				attrs := EdgeAttrs{A2B: 0, Last: 0, Threshold: threshold}
				edge := Edge{FromNodeId: node.Id, ToNodeId: item.Id, Attrs: attrs}
				err := graph.AddEdge(edge)
				if err != nil {
					return nil, err
				}
				// graph.SetEdgeAttributes()
			}
		}
	}

	fmt.Println("Graph network is created.")
	return graph, nil
}

func isThresholdFailed(firstNodeId int, secondNodeId int, chunkId int, g *Graph) bool {
	if Constants.GetThresholdEnabled() {
		edgeDataFirst := g.GetEdgeData(firstNodeId, secondNodeId)
		p2pFirst := edgeDataFirst.A2B
		edgeDataSecond := g.GetEdgeData(secondNodeId, firstNodeId)
		p2pSecond := edgeDataSecond.A2B

		threshold := Constants.GetThreshold()
		if Constants.IsAdjustableThreshold() {
			threshold = edgeDataFirst.Threshold
		}
		price := p2pFirst - p2pSecond + PeerPriceChunk(secondNodeId, chunkId)
		fmt.Printf("price: %d ", price)
		return price > threshold
	}
	return false
}

func getNext(firstNode *Node, chunkId int, graph *Graph, mainOriginatorId int, prevNodePaid bool, rerouteMap RerouteMap) (int, *Node, []Threshold, bool, bool, Payment, bool) {
	var nextNode *Node = nil
	var payNext *Node = nil
	var thresholdList []Threshold
	var thresholdFailed bool
	var accessFailed bool
	var payment Payment
	resultInt := 1
	lastDistance := firstNode.Id ^ chunkId
	fmt.Printf("\n last distance is : %d, chunk is: %d, first is: %d", lastDistance, chunkId, firstNode.Id)
	fmt.Printf("\n which bucket: %d \n", 16-BitLength(chunkId^firstNode.Id))

	currDist := lastDistance
	payDist := lastDistance
	for _, adj := range firstNode.Adj {
		for _, node := range adj {
			dist := node.Id ^ chunkId
			if BitLength(dist) >= BitLength(lastDistance) {
				continue
			}

			if !isThresholdFailed(firstNode.Id, node.Id, chunkId, graph) {
				thresholdFailed = false

				// Could probably clean this one up, but keeping it close to original for now
				if dist < currDist {
					if Constants.IsRetryWithAnotherPeer() {
						_, ok := rerouteMap[mainOriginatorId]
						if ok {
							allExceptLast := len(rerouteMap[mainOriginatorId]) - 1
							if Contains(rerouteMap[mainOriginatorId][:allExceptLast], node.Id) {
								continue
							} else {
								currDist = dist
								nextNode = node
							}
						} else {
							currDist = dist
							nextNode = node
						}
					} else {
						currDist = dist
						nextNode = node
					}
				}
			} else {
				thresholdFailed = true
				if Constants.GetPaymentEnabled() {
					if dist < payDist {
						payDist = dist
						payNext = node
					}
				}
				listItem := Threshold{firstNode, node}
				thresholdList = append(thresholdList, listItem)
			}
		}
	}
	if nextNode != nil {
		thresholdFailed = false
		accessFailed = false
	} else {
		if !thresholdFailed {
			accessFailed = true
			resultInt = -2
			// nextNode = -2 // accessFailed, TYPE MISMATCH ??
		} else {
			resultInt = -1
			// nextNode = -1 // thresholdFailed, TYPE MISMATCH ??
		}
		if Constants.GetPaymentEnabled() {
			if payNext != nil {
				accessFailed = false
				if Constants.IsOnlyOriginatorPays() {
					if firstNode.Id == mainOriginatorId {
						payment.IsOriginator = true
						payment.FirstNodeId = firstNode.Id
						payment.PayNextId = payNext.Id
						payment.ChunkId = chunkId
						nextNode = payNext
					} else {
						thresholdFailed = true
						resultInt = -1
					}
				} else if Constants.IsPayIfOrigPays() {
					if prevNodePaid {
						nextNode = payNext
						thresholdFailed = false
						if firstNode.Id == mainOriginatorId {
							payment.IsOriginator = true
						} else {
							payment.IsOriginator = false
						}
						payment.FirstNodeId = firstNode.Id
						payment.PayNextId = payNext.Id
						payment.ChunkId = chunkId
					} else {
						if firstNode.Id == mainOriginatorId {
							payment.IsOriginator = true
							payment.FirstNodeId = firstNode.Id
							payment.PayNextId = payNext.Id
							payment.ChunkId = chunkId
							nextNode = payNext
						} else {
							thresholdFailed = true
							resultInt = -1
							payNext = nil
						}
					}
				} else {
					nextNode = payNext
					thresholdFailed = false
					if firstNode.Id == mainOriginatorId {
						payment.IsOriginator = true
					} else {
						payment.IsOriginator = false
					}
					payment.FirstNodeId = firstNode.Id
					payment.PayNextId = payNext.Id
					payment.ChunkId = chunkId
				}
			}
		}
	}
	// TODO: Usikker på dette
	if Constants.GetPaymentEnabled() {
	out:
		for i, item := range thresholdList {
			for _, node := range item {
				if node == payNext {
					if Constants.IsPayIfOrigPays() {
						if firstNode.Id == mainOriginatorId {
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
	if payment != (Payment{}) {
		prevNodePaid = true
	} else {
		prevNodePaid = false
	}
	// RASMUS: nil reference error
	if nextNode != nil {
		fmt.Printf("\n next node is: %d", nextNode.Id)
	}
	return resultInt, nextNode, thresholdList, thresholdFailed, accessFailed, payment, prevNodePaid
}

// ConsumeTask cacheDict is map of nodes containing an array of maps with key as a chunkAddr and a popularity counter
func ConsumeTask(request *Request, graph *Graph, respNodes []int, rerouteMap RerouteMap, cacheListMap CacheListMap) (bool, Route, [][]Threshold, bool, []Payment) {
	var thresholdFailedList [][]Threshold
	var paymentList []Payment
	originator := request.Originator
	chunkId := request.ChunkId
	mainOriginator := originator
	found := false
	foundByCaching := false
	route := Route{originator.Id}
	var resultInt int
	var nextNode *Node
	var thresholdList []Threshold
	// thresholdFailed := false
	var accessFailed bool
	var payment Payment
	var prevNodePaid bool
	if Constants.IsPayIfOrigPays() {
		prevNodePaid = true
	}
	if Contains(respNodes, originator.Id) {
		// originator has the chunk
		found = true
	} else {
	out:
		for _, node := range respNodes {
			fmt.Printf("\n orig: %d, chunk_id: %d", originator.Id, chunkId)
			if node != originator.Id {
				// nextNode, thresholdList, thresholdFailed, accessFailed, payment, prevNodePaid = getNext(originator, chunkId, graph, mainOriginator, prevNodePaid, rerouteMap)
				resultInt, nextNode, thresholdList, _, accessFailed, payment, prevNodePaid = getNext(originator, chunkId, graph, mainOriginator.Id, prevNodePaid, rerouteMap)
				if payment != (Payment{}) {
					paymentList = append(paymentList, payment)
				}
				if len(thresholdList) > 0 {
					thresholdFailedList = append(thresholdFailedList, thresholdList)
				}
				// RASMUS: Nil reference error
				if nextNode != nil {
					route = append(route, nextNode.Id)
				}
				// if not isinstance(next_node, int), originale versjonen
				if !(resultInt <= -1) && nextNode != nil {
					if Contains(respNodes, nextNode.Id) {
						fmt.Println("is not in cache")
						found = true
						break out
					}
					cacheList, ok := cacheListMap[nextNode]
					if ok {
						for _, cacheMap := range cacheList {
							_, ok2 := cacheMap[chunkId]
							if ok2 {
								fmt.Println("is in cache")
								found = true
								foundByCaching = true
								break out
							}
						}
					}
					originator = nextNode
				} else {
					break out
				}
			}
		}
	}
	route = append(route, chunkId)

	if Constants.IsForwarderPayForceOriginatorToPay() {
		// if !Contains(route, -2) { // Gir ikke mening lengre
		if resultInt != -2 {
			// NOT accessFailed
			if len(paymentList) > 0 {
				firstPayment := paymentList[0]
				if !firstPayment.IsOriginator {
					// TODO: Dobbelsjekk at logikken under her matcher originalen
					for i := range route[:len(route)-1] {
						p := Payment{route[i], route[i+1], route[len(route)-1], false}
						for j, tmp := range paymentList {
							if p.PayNextId == tmp.PayNextId && p.FirstNodeId == tmp.FirstNodeId && p.ChunkId == tmp.ChunkId {
								break
							}
							if j == len(paymentList) {
								// payment is now definitely not in paymentList
								if i == 0 {
									p.IsOriginator = true
								}
								if i != len(route)-2 {
									paymentList = append(paymentList[:i+1], paymentList[i:]...)
									paymentList[i] = p
								} else {
									continue
								}
							}
						}
					}
				} else {
					// TODO: Dobbelsjekk at logikken under her matcher originalen
					for i := range route[1 : len(route)-1] {
						p := Payment{route[i], route[i+1], route[len(route)-1], false}
						for j, tmp := range paymentList {
							if p.PayNextId == tmp.PayNextId && p.FirstNodeId == tmp.FirstNodeId && p.ChunkId == tmp.ChunkId {
								break
							}
							if j == len(paymentList) {
								// payment is now definitely not in paymentList
								if i != len(route)-2 {
									paymentList = append(paymentList[:i+1], paymentList[i:]...)
									paymentList[i] = p
								} else {
									continue
								}
							}
						}
					}
				}
			}
		} else {
			paymentList = []Payment{}
		}
	}
	if foundByCaching {
		// route = append(route, "C") // TYPE MISMATCH
		route = append(route, -3) // TODO: midlertidig fix?
	}
	return found, route, thresholdFailedList, accessFailed, paymentList
}

func getProximityChunk(firstNodeId int, chunkId int) int {
	retVal := Constants.GetBits() - BitLength(firstNodeId^chunkId)
	if retVal <= Constants.GetMaxProximityOrder() {
		return retVal
	} else {
		return Constants.GetMaxProximityOrder()
	}
}

func PeerPriceChunk(firstNodeId int, chunkId int) int {
	val := (Constants.GetMaxProximityOrder() - getProximityChunk(firstNodeId, chunkId) + 1) * Constants.GetPrice()
	return val
}

func CreateDownloadersList(g *Graph) []int {
	fmt.Println("Creating downloaders list...")

	nodesValue := make([]int, 0, len(g.Nodes))
	for _, v := range g.Nodes {
		nodesValue = append(nodesValue, v.Id)
	}
	downloadersList := Choice(nodesValue, Constants.GetOriginators())

	fmt.Println("Downloaders list create...!")
	return downloadersList
}

func CreateNodesList(g *Graph) []int {
	fmt.Println("Creating nodes list...")

	nodesValue := make([]int, 0, len(g.Nodes))
	for _, v := range g.Nodes {
		nodesValue = append(nodesValue, v.Id)
	}

	fmt.Println("Nodes list create...!")
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
//	for i := 0; i <= ct.Constants.GetOriginators(); i++ {
//		// chunksList := choice(ct.Constants.GetChunks(), ct.Constants.GetRangeAddress())
//		// filesList = append(chunksList)
//		fmt.Println(i)
//	}
//	// Gets all constants
//	consts := ct.Constants
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
