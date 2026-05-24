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

// StartWorkflow godoc
// @Summary      Start a workflow
// @Description  Create and immediately start a multi-step workflow
// @Tags         workflows
// @Accept       json
// @Produce      json
// @Param        workflow body service.CreateWorkflowInput true "Workflow input"
// @Success      201  {object}  repository.Workflow
// @Failure      400  {object}  map[string]string
// @Router       /workflows [post]
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

// GetWorkflow godoc
// @Summary      Get workflow with step details
// @Tags         workflows
// @Produce      json
// @Param        id   path      string  true  "Workflow ID"
// @Success      200  {object}  map[string]any
// @Failure      404  {object}  map[string]string
// @Router       /workflows/{id} [get]
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

// GetAllWorkflows godoc
// @Summary      List all workflows
// @Tags         workflows
// @Produce      json
// @Success      200  {object}  map[string]any
// @Router       /workflows [get]
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
