package services

import (
	"errors"
	"workflow-engine/models"
)

// TopoSort performs a topological sort on the workflow steps.
// It returns a 2D slice where each inner slice represents a group of steps that can be executed in parallel.
// It also checks for cycles in the DAG.
func TopoSort(definition models.WorkflowDefinition) ([][]models.WorkflowStep, error) {
	steps := definition.Steps
	if len(steps) == 0 {
		return nil, errors.New("workflow has no steps")
	}

	stepMap := make(map[string]models.WorkflowStep)
	inDegree := make(map[string]int)
	adjList := make(map[string][]string) // adjList[id] = list of step IDs that depend on id

	// Initialize
	for _, step := range steps {
		stepMap[step.ID] = step
		inDegree[step.ID] = len(step.DependsOn)
		if _, exists := adjList[step.ID]; !exists {
			adjList[step.ID] = []string{}
		}
	}

	// Build Adjacency List and In-Degree
	for _, step := range steps {
		for _, depID := range step.DependsOn {
			if _, exists := stepMap[depID]; !exists {
				return nil, errors.New("step " + step.ID + " depends on non-existent step " + depID)
			}
			adjList[depID] = append(adjList[depID], step.ID)
		}
	}

	var result [][]models.WorkflowStep
	var queue []string

	// Find nodes with 0 in-degree
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	processedCount := 0

	for len(queue) > 0 {
		var nextQueue []string
		var currentLayer []models.WorkflowStep

		// Process all nodes in the current queue (these can be run in parallel)
		for _, id := range queue {
			currentLayer = append(currentLayer, stepMap[id])
			processedCount++

			for _, neighbor := range adjList[id] {
				inDegree[neighbor]--
				if inDegree[neighbor] == 0 {
					nextQueue = append(nextQueue, neighbor)
				}
			}
		}

		result = append(result, currentLayer)
		queue = nextQueue
	}

	if processedCount != len(steps) {
		return nil, errors.New("cycle detected in workflow DAG")
	}

	return result, nil
}
