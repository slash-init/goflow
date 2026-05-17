package main

import (
    "log"
    "github.com/gofiber/fiber/v2"
    "github.com/slash-init/goflow/internal/api"
)

func main() {
   //create a new Fiber app
   app := fiber.New()

   //register a route
   app.Get("/health", api.HealthHandler)

   // start server
   log.Fatal(app.Listen(":8080"))
}
