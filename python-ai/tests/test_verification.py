import pytest
from fastapi.testclient import TestClient
from main import app

client = TestClient(app)

def test_verify_report():
    response = client.post("/verify", json={
        "report_id": "test-123",
        "image_url": "http://example.com/pothole.jpg",
        "description": "Large pothole on main road",
        "category": "pothole"
    })
    
    assert response.status_code == 200
    data = response.json()
    assert "verified" in data
    assert "severity" in data

def test_health_check():
    response = client.get("/health")
    assert response.status_code == 200
    assert response.json()["status"] == "healthy"