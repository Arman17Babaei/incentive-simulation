from collections import Counter
import json
import re
from typing import Dict, List

PATH = "network_data/nodes_data_b13_k8_8192_.txt"
with open(PATH, "r") as f:
    data = json.load(f)

print("bits:", data["bits"])
print("bins:", data["bin"])

adj: Dict[int, List[int]] = {}

for node in data["nodes"]:
    id = node["id"]
    adj[id] = []
    for neighbor in node["adj"]:
        adj[id].append(neighbor)

dist_from_0 = {}
dist_from_0[0] = 0
queue = [0]
while len(queue) > 0:
    node = queue.pop(0)
    for neighbor in adj[node]:
        if neighbor not in dist_from_0:
            dist_from_0[neighbor] = dist_from_0[node] + 1
            queue.append(neighbor)

income = {}
with open("results/income.txt", "r") as f:
    for line in f:
        regex = r"(\d+):(\d+)"
        if not re.match(regex, line):
            continue
        line = line.split(":")
        income[int(line[0])] = int(line[1])


def print_table():
    for i in range(1 << data["bits"]):
        if i not in income:
            print(i, "not in income")
            continue
        if i not in dist_from_0:
            print(i, "not in dist_from_0")
            continue
        print(i, income[i], dist_from_0[i])


dist_income = {}
for i in range(1 << data["bits"]):
    if i not in income:
        continue
    if i not in dist_from_0:
        continue
    if dist_from_0[i] not in dist_income:
        dist_income[dist_from_0[i]] = []
    dist_income[dist_from_0[i]].append(income[i])


def plot_distance_based_income():
    # plot histograms based on distance
    import matplotlib.pyplot as plt

    fig, axs = plt.subplots(3, 1)
    fig.tight_layout()
    for dist in dist_income:
        axs[dist - 1].hist(dist_income[dist], bins=100)
        axs[dist - 1].set_ylim(0, 50)
        axs[dist - 1].set_title(f"Distance {dist}")

    plt.show()


def msb(x: int):
    return x.bit_length() - 1

bins_0 = {}
for node in adj[0]:
    if msb(node) not in bins_0:
        bins_0[msb(node)] = []
    bins_0[msb(node)].append(node)

for bin_ in bins_0:
    incomes = []
    for node in bins_0[bin_]:
        incomes.append((income[node], node))
    print(f"bin {bin_}:")
    print(*map(lambda x: x[0], incomes), sep='\t')
    print(*map(lambda x: bin(x[1])[2:], incomes), sep='\t')
print(*bins_0.items(), sep="\n")