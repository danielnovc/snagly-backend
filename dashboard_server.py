#!/usr/bin/env python3
"""
Simple web server to serve the Distrack monitoring dashboard
"""
from http.server import HTTPServer, SimpleHTTPRequestHandler
import socketserver
import os
import sys

class CORSRequestHandler(SimpleHTTPRequestHandler):
    def end_headers(self):
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Access-Control-Allow-Methods', 'GET, POST, OPTIONS')
        self.send_header('Access-Control-Allow-Headers', 'Content-Type')
        super().end_headers()

    def do_OPTIONS(self):
        self.send_response(200)
        self.end_headers()

def run_server(port=3000):
    """Run the web server on the specified port"""
    os.chdir(os.path.dirname(os.path.abspath(__file__)))
    
    with socketserver.TCPServer(("", port), CORSRequestHandler) as httpd:
        print(f"🚀 Dashboard server running on http://localhost:{port}")
        print(f"📊 Open your browser and go to: http://localhost:{port}/real_time_dashboard.html")
        print(f"📋 Or use the static version: http://localhost:{port}/monitoring_dashboard.html")
        print("Press Ctrl+C to stop the server")
        
        try:
            httpd.serve_forever()
        except KeyboardInterrupt:
            print("\n🛑 Server stopped")

if __name__ == "__main__":
    port = 3000
    if len(sys.argv) > 1:
        try:
            port = int(sys.argv[1])
        except ValueError:
            print("Invalid port number. Using default port 3000")
    
    run_server(port)
