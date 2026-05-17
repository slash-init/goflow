package main

import (
    "log"
    "github.com/gofiber/fiber/v2"
)

func main() {
   //create a new Fiber app
   app := fiber.New()

   //register a route
   app.Get("/health", func(c *fiber.Ctx) error {
       return c.JSON(fiber.Map{
           "status":  "ok",
           "service": "goflow",
       })
   })

   // start server
   log.Fatal(app.Listen(":8080"))
}
