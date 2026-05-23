package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/slash-init/goflow/internal/service"
)

type WorkflowHandler struct {
	svc *service.WorkflowService
}

func NewWorkflowHandler(svc *service.WorkflowService) *WorkflowHandler {
	return &WorkflowHandler{svc: svc}
}

func (h *WorkflowHandler) StartWorkflow(c *fiber.Ctx) error {
	var input service.CreateWorkflowInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	if input.Name == "" {
		input.Name = "order:process"
	}

	wf, err := h.svc.StartOrderWorkflow(c.Context(), input)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(wf)
}

func (h *WorkflowHandler) GetWorkflow(c *fiber.Ctx) error {
	id := c.Params("id")
	wf, steps, err := h.svc.GetWorkflow(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"workflow": wf,
		"steps":    steps,
	})
}

func (h *WorkflowHandler) GetAllWorkflows(c *fiber.Ctx) error {
	workflows, err := h.svc.GetAllWorkflows()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "could not fetch workflows",
		})
	}

	return c.JSON(fiber.Map{
		"count":     len(workflows),
		"workflows": workflows,
	})
}
