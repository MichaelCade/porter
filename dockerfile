FROM golang:1.22 AS builder
WORKDIR /app
COPY . .
RUN go build -o porter .

FROM debian:stable

# Install dependencies
RUN apt-get update && \
    apt-get install -y qemu-utils curl unzip python3 python3-venv python3-pip && \
    apt-get install -y awscli && \
    curl -sL https://aka.ms/InstallAzureCLIDeb | bash && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/porter /usr/local/bin/porter
COPY --from=builder /app/simple_template.html /app/simple_template.html

# Create directories for extracted/converted files (to mount volumes)
RUN mkdir -p /app/extracted /app/converted

# Set directory permissions
RUN chmod 777 /app/extracted /app/converted

# Expose web app on 8080
EXPOSE 8080
CMD ["porter"]
