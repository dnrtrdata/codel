package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.45

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/semanser/ai-coder/config"
	"github.com/semanser/ai-coder/database"
	"github.com/semanser/ai-coder/executor"
	gmodel "github.com/semanser/ai-coder/graph/model"
	"github.com/semanser/ai-coder/graph/subscriptions"
	"github.com/semanser/ai-coder/websocket"
)

// CreateFlow is the resolver for the createFlow field.
func (r *mutationResolver) CreateFlow(ctx context.Context, modelProvider string, modelID string) (*gmodel.Flow, error) {
	if modelID == "" || modelProvider == "" {
		return nil, fmt.Errorf("model is required")
	}

	flow, err := r.Db.CreateFlow(ctx, database.CreateFlowParams{
		Name:          database.StringToNullString("New Task"),
		Status:        database.StringToNullString("in_progress"),
		Model:         database.StringToNullString(modelID),
		ModelProvider: database.StringToNullString(modelProvider),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create flow: %w", err)
	}

	executor.AddQueue(int64(flow.ID), r.Db)

	return &gmodel.Flow{
		ID:     uint(flow.ID),
		Name:   flow.Name.String,
		Status: gmodel.FlowStatus(flow.Status.String),
		Model: &gmodel.Model{
			Provider: flow.ModelProvider.String,
			ID:       flow.Model.String,
		},
	}, nil
}

// CreateTask is the resolver for the createTask field.
func (r *mutationResolver) CreateTask(ctx context.Context, flowID uint, query string) (*gmodel.Task, error) {
	type InputTaskArgs struct {
		Query string `json:"query"`
	}

	args := InputTaskArgs{Query: query}
	arg, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}

	task, err := r.Db.CreateTask(ctx, database.CreateTaskParams{
		Type:    database.StringToNullString("input"),
		Message: database.StringToNullString(query),
		Status:  database.StringToNullString("finished"),
		Args:    database.StringToNullString(string(arg)),
		FlowID:  sql.NullInt64{Int64: int64(flowID), Valid: true},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	executor.AddCommand(int64(flowID), task)

	return &gmodel.Task{
		ID:        uint(task.ID),
		Message:   task.Message.String,
		Type:      gmodel.TaskType(task.Type.String),
		Status:    gmodel.TaskStatus(task.Status.String),
		Args:      database.StringToNullString(string(arg)).String,
		CreatedAt: task.CreatedAt.Time,
	}, nil
}

// FinishFlow is the resolver for the finishFlow field.
func (r *mutationResolver) FinishFlow(ctx context.Context, flowID uint) (*gmodel.Flow, error) {
	// Remove all tasks from the queue
	executor.CleanQueue(int64(flowID))

	go func() {
		// Delete the docker container
		flow, err := r.Db.ReadFlow(context.Background(), int64(flowID))

		if err != nil {
			log.Printf("Error reading flow: %s\n", err)
		}

		err = executor.DeleteContainer(flow.ContainerLocalID.String, flow.ContainerID.Int64, r.Db)

		if err != nil {
			log.Printf("Error deleting container: %s\n", err)
		}
	}()

	// Update flow status
	r.Db.UpdateFlowStatus(ctx, database.UpdateFlowStatusParams{
		Status: database.StringToNullString("finished"),
		ID:     int64(flowID),
	})

	// Broadcast flow update
	subscriptions.BroadcastFlowUpdated(int64(flowID), &gmodel.Flow{
		ID:     flowID,
		Status: gmodel.FlowStatus("finished"),
	})

	return &gmodel.Flow{
		ID:     flowID,
		Status: gmodel.FlowStatus("finished"),
	}, nil
}

// Exec is the resolver for the _exec field.
func (r *mutationResolver) Exec(ctx context.Context, containerID string, command string) (string, error) {
	b := bytes.Buffer{}
	// executor.ExecCommand(containerID, command, &b)

	return b.String(), nil
}

// AvailableModels is the resolver for the availableModels field.
func (r *queryResolver) AvailableModels(ctx context.Context) ([]*gmodel.Model, error) {
	var availableModels []*gmodel.Model

	if config.Config.OpenAIKey != "" && config.Config.OpenAIModel != "" {
		availableModels = append(availableModels, &gmodel.Model{
			Provider: "openai",
			ID:       config.Config.OpenAIModel,
		})
	}

	if config.Config.OllamaModel != "" {
		availableModels = append(availableModels, &gmodel.Model{
			Provider: "ollama",
			ID:       config.Config.OllamaModel,
		})
	}

	return availableModels, nil
}

// Flows is the resolver for the flows field.
func (r *queryResolver) Flows(ctx context.Context) ([]*gmodel.Flow, error) {
	flows, err := r.Db.ReadAllFlows(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch flows: %w", err)
	}

	var gFlows []*gmodel.Flow

	for _, flow := range flows {
		var gTasks []*gmodel.Task
		var logs []*gmodel.Log

		gFlows = append(gFlows, &gmodel.Flow{
			ID:   uint(flow.ID),
			Name: flow.Name.String,
			Terminal: &gmodel.Terminal{
				ContainerName: flow.ContainerName.String,
				Connected:     false,
				Logs:          logs,
			},
			Tasks:  gTasks,
			Status: gmodel.FlowStatus(flow.Status.String),
			Model: &gmodel.Model{
				Provider: flow.ModelProvider.String,
				ID:       flow.Model.String,
			},
		})
	}

	return gFlows, nil
}

// Flow is the resolver for the flow field.
func (r *queryResolver) Flow(ctx context.Context, id uint) (*gmodel.Flow, error) {
	flow, err := r.Db.ReadFlow(ctx, int64(id))

	if err != nil {
		return nil, fmt.Errorf("failed to fetch flow: %w", err)
	}

	var gFlow *gmodel.Flow
	var gTasks []*gmodel.Task
	var gLogs []*gmodel.Log

	tasks, err := r.Db.ReadTasksByFlowId(ctx, sql.NullInt64{Int64: int64(id), Valid: true})

	if err != nil {
		return nil, fmt.Errorf("failed to fetch tasks: %w", err)
	}

	for _, task := range tasks {
		gTasks = append(gTasks, &gmodel.Task{
			ID:        uint(task.ID),
			Message:   task.Message.String,
			Type:      gmodel.TaskType(task.Type.String),
			Status:    gmodel.TaskStatus(task.Status.String),
			Args:      task.Args.String,
			Results:   task.Results.String,
			CreatedAt: task.CreatedAt.Time,
		})
	}
	logs, err := r.Db.GetLogsByFlowId(ctx, sql.NullInt64{Int64: flow.ID, Valid: true})

	if err != nil {
		return nil, fmt.Errorf("failed to fetch logs: %w", err)
	}

	for _, log := range logs {
		text := log.Message

		if log.Type == "input" {
			text = websocket.FormatTerminalInput(log.Message)
		}

		gLogs = append(gLogs, &gmodel.Log{
			ID:   uint(log.ID),
			Text: text,
		})
	}

	gFlow = &gmodel.Flow{
		ID:    uint(flow.ID),
		Name:  flow.Name.String,
		Tasks: gTasks,
		Terminal: &gmodel.Terminal{
			ContainerName: flow.ContainerName.String,
			Connected:     flow.ContainerStatus.String == "running",
			Logs:          gLogs,
		},
		Status: gmodel.FlowStatus(flow.Status.String),
		Browser: &gmodel.Browser{
			URL:           "",
			ScreenshotURL: "",
		},
		Model: &gmodel.Model{
			Provider: flow.ModelProvider.String,
			ID:       flow.Model.String,
		},
	}

	return gFlow, nil
}

// TaskAdded is the resolver for the taskAdded field.
func (r *subscriptionResolver) TaskAdded(ctx context.Context, flowID uint) (<-chan *gmodel.Task, error) {
	return subscriptions.TaskAdded(ctx, int64(flowID))
}

// TaskUpdated is the resolver for the taskUpdated field.
func (r *subscriptionResolver) TaskUpdated(ctx context.Context) (<-chan *gmodel.Task, error) {
	panic(fmt.Errorf("not implemented: TaskUpdated - taskUpdated"))
}

// FlowUpdated is the resolver for the flowUpdated field.
func (r *subscriptionResolver) FlowUpdated(ctx context.Context, flowID uint) (<-chan *gmodel.Flow, error) {
	return subscriptions.FlowUpdated(ctx, int64(flowID))
}

// BrowserUpdated is the resolver for the browserUpdated field.
func (r *subscriptionResolver) BrowserUpdated(ctx context.Context, flowID uint) (<-chan *gmodel.Browser, error) {
	return subscriptions.BrowserUpdated(ctx, int64(flowID))
}

// TerminalLogsAdded is the resolver for the terminalLogsAdded field.
func (r *subscriptionResolver) TerminalLogsAdded(ctx context.Context, flowID uint) (<-chan *gmodel.Log, error) {
	return subscriptions.TerminalLogsAdded(ctx, int64(flowID))
}

// Mutation returns MutationResolver implementation.
func (r *Resolver) Mutation() MutationResolver { return &mutationResolver{r} }

// Query returns QueryResolver implementation.
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

// Subscription returns SubscriptionResolver implementation.
func (r *Resolver) Subscription() SubscriptionResolver { return &subscriptionResolver{r} }

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
type subscriptionResolver struct{ *Resolver }
