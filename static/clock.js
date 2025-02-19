document.addEventListener("DOMContentLoaded", function () {
    const statusMessage = document.getElementById("status-message");

    // Connect to WebSocket for real-time updates
    const socket = new WebSocket("ws://localhost:8080/ws/device");

    socket.onmessage = function (event) {
        const data = JSON.parse(event.data);
        if (data.status === "success") {
            statusMessage.textContent = "Card scanned successfully!";
        } else {
            statusMessage.textContent = "Error: " + data.message;
        }
    };

    socket.onerror = function (error) {
        statusMessage.textContent = "WebSocket error: " + error.message;
    };
});