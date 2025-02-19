document.addEventListener("DOMContentLoaded", function () {
    const statusMessage = document.getElementById("status-message");
    const employeeName = document.getElementById("employee-name");
    const lastClockIn = document.getElementById("last-clock-in");
    const lastClockOut = document.getElementById("last-clock-out");

    // Connect to WebSocket for real-time updates
    const socket = new WebSocket("ws://localhost:8080/ws/device");

    socket.onmessage = function (event) {
        const data = JSON.parse(event.data);

        if (data.status === "success") {
            // Update status message
            statusMessage.textContent = "Card scanned successfully!";

            // Update employee information
            if (data.employee) {
                employeeName.textContent = data.employee.name || "-";
                lastClockIn.textContent = data.employee.last_clock_in
                    ? new Date(data.employee.last_clock_in).toLocaleString()
                    : "-";
                lastClockOut.textContent = data.employee.last_clock_out
                    ? new Date(data.employee.last_clock_out).toLocaleString()
                    : "-";
            }
        } else {
            // Show error message
            statusMessage.textContent = "Error: " + data.message;
        }
    };

    socket.onerror = function (error) {
        statusMessage.textContent = "WebSocket error: " + error.message;
    };

    socket.onclose = function () {
        statusMessage.textContent = "Connection closed. Please refresh the page.";
    };
});
