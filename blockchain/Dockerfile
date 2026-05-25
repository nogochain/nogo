# Use the official Go image as a base
FROM golang:1.26-alpine

# Set the current working directory inside the container
WORKDIR /app

# Copy the application source code
COPY . .

# Build the Go application
RUN go build -o nogo .

# Expose the port the blockchain server will run on
EXPOSE 8080

# Command to run the executable
CMD ["./nogo"]
