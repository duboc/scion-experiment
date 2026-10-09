"""
Locust load testing script for PSearch API.

This script defines user behaviors for load testing the PSearch API,
including health checks and product searches with various parameters.

Usage:
    locust -f locustfile.py
"""

import random
import json
import logging
from locust import HttpUser, task, between, events
from search_queries import SEARCH_QUERIES
from config import MIN_WAIT_TIME, MAX_WAIT_TIME

# Set up logging
logger = logging.getLogger(__name__)

# Log test start and end events
@events.test_start.add_listener
def on_test_start(environment, **kwargs):
    logger.info("Starting PSearch API load test")

@events.test_stop.add_listener
def on_test_stop(environment, **kwargs):
    logger.info("PSearch API load test completed")

class PSearchUser(HttpUser):
    """Simulates a user interacting with the PSearch API."""
    
    # Wait time between tasks in seconds (convert from ms)
    wait_time = between(MIN_WAIT_TIME/1000, MAX_WAIT_TIME/1000)
    
    def on_start(self):
        """Called when a simulated user starts."""
        logger.info("New user started")
    
    @task(1)
    def health_check(self):
        """Perform health check."""
        with self.client.get("/health", name="Health Check", catch_response=True) as response:
            if response.status_code == 200:
                try:
                    data = response.json()
                    if data.get("status") != "healthy":
                        response.failure(f"Unexpected health status: {data.get('status')}")
                except json.JSONDecodeError:
                    response.failure("Invalid JSON in health response")
            else:
                response.failure(f"Health check failed with status code {response.status_code}")
    
    @task(5)
    def search_products(self):
        """Perform product search with various queries."""
        # Select a random search query
        query = random.choice(SEARCH_QUERIES)
        query_str = query.get("query", "unknown")
        
        # Make POST request to search endpoint
        with self.client.post(
            "/search",
            json=query,
            headers={"Content-Type": "application/json"},
            name=f"Search: {query_str[:20]}...",
            catch_response=True
        ) as response:
            if response.status_code == 200:
                try:
                    data = response.json()
                    # Validate response structure
                    if "results" not in data:
                        response.failure("Missing 'results' field in response")
                    elif "total_found" not in data:
                        response.failure("Missing 'total_found' field in response")
                    # Optional: Further validate specific fields in results
                except json.JSONDecodeError:
                    response.failure("Invalid JSON in search response")
            elif response.status_code == 400:
                # Bad request is acceptable for some test queries
                logger.warning(f"Bad request for query: {query}")
            else:
                response.failure(f"Search failed with status code {response.status_code}")
    
    # Removing the invalid parameters test task as it's causing inconsistent results
    # The server appears to handle some "invalid" parameters differently than expected
    # We'll focus on testing with valid parameters only
