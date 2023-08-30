package utils

import (
	"go-incentive-simulation/config"
	"go-incentive-simulation/model/general"
	"go-incentive-simulation/model/parts/types"
	"math/rand"
	"sort"
)

func SortedKeys(nodeMap map[types.NodeId]*types.Node) []types.NodeId {
	keys := make([]types.NodeId, len(nodeMap))
	i := 0
	for k := range nodeMap {
		keys[i] = k
		i++
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	if keys[0] < 0 {
		panic("generated network contains a node with an invalid Id")
	}

	return keys
}

func CreateGraphNetwork(net *types.Network) (*types.Graph, error) {
	//fmt.Println("Creating graph network...")
	sortedNodeIds := SortedKeys(net.NodesMap)

	Edges := make(map[types.NodeId]map[types.NodeId]*types.Edge)

	graph := &types.Graph{
		Network: net,
		Edges:   Edges,
	}

	for _, nodeId := range sortedNodeIds {
		graph.Edges[nodeId] = make(map[types.NodeId]*types.Edge)

		node := net.NodesMap[nodeId]
		nodeAdj := node.AdjIds
		for _, adjItems := range nodeAdj {
			for _, otherNodeId := range adjItems {
				threshold := general.BitLength(nodeId.ToInt() ^ otherNodeId.ToInt())
				attrs := types.EdgeAttrs{A2B: 0, LastEpoch: 0, Threshold: threshold}
				err := graph.AddEdge(node.Id, otherNodeId, attrs)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	//fmt.Println("Graph network is created.")
	return graph, nil
}

func GetNewChunkId() types.ChunkId {
	return types.ChunkId(rand.Intn(config.GetAddressRange()-1) + 1)
}

func GetPreferredChunkId() types.ChunkId {
	var chunkId types.ChunkId
	var random float32
	numPreferredChunks := 1
	random = rand.Float32()
	if float32(random) <= 0.8 {
		chunkId = types.ChunkId(rand.Intn(numPreferredChunks))
	} else {
		chunkId = types.ChunkId(rand.Intn(config.GetAddressRange()-numPreferredChunks) + numPreferredChunks)
	}
	return chunkId
}

func FindDistance(first types.NodeId, second types.ChunkId) int {
	return config.GetBits() - general.BitLength(first.ToInt()^second.ToInt())
}

func getProximityChunk(firstNodeId types.NodeId, chunkId types.ChunkId) int {
	retVal := config.GetBits() - general.BitLength(firstNodeId.ToInt()^chunkId.ToInt())
	if retVal <= config.GetMaxProximityOrder() {
		return retVal
	} else {
		return config.GetMaxProximityOrder()
	}
}

func PeerPriceChunk(firstNodeId types.NodeId, chunkId types.ChunkId) int {
	val := (config.GetMaxProximityOrder() - getProximityChunk(firstNodeId, chunkId) + 1) * config.GetPrice()
	return val
}

func CreateDownloadersList(g *types.Graph) []types.NodeId {
	//fmt.Println("Creating downloaders list...")

	downloadersList := make([]types.NodeId, 0)

	if config.GetOriginators() == 1 {
		downloadersList = append(downloadersList, 0)
		return downloadersList
	}

	counter := 0
	for _, originator := range g.NodesMap {
		downloadersList = append(downloadersList, originator.Id)
		counter++
		if counter >= config.GetOriginators() {
			break
		}
	}

	//fmt.Println("Downloaders list create...!")
	return downloadersList
}
