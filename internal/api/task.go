package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/slash-init/goflow/internal/service"
)

type TaskHandler struct {
	svc *service.TaskService
}

func NewTaskHandler(svc *service.TaskService) *TaskHandler {
	return &TaskHandler{svc: svc}
}

func (h *TaskHandler) CreateTask(c *fiber.Ctx) error {
	var input service.CreateTaskInput

	// Parse request body
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	task, err := h.svc.CreateTask(input)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(task)
}

func (h *TaskHandler) GetAllTasks(c *fiber.Ctx) error {
	tasks, err := h.svc.GetAllTaks()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "could not fetch tasks",
		})
	}

	return c.JSON(fiber.Map{
		"tasks": tasks,
		"count": len(tasks),
	})
}

func (h *TaskHandler) GetTaskByID(c *fiber.Ctx) error {
    id := c.Params("id")     // like req.params.id in Express

    task, err := h.svc.GetTaskByID(id)
    if err != nil {
        return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
            "error": err.Error(),
        })
    }

    return c.JSON(task)
}