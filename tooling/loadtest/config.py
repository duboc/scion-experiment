import os
from dotenv import load_dotenv

# Load environment variables from .env file
load_dotenv()

# API host with fallback values
DEFAULT_HOST = os.getenv("API_HOST", "https://product-search-api-1234567890.us-central1.run.app")
LOCAL_HOST = os.getenv("LOCAL_API_HOST", "http://localhost:8080")

# Test parameters
MIN_WAIT_TIME = int(os.getenv("MIN_WAIT_TIME", 1000))  # milliseconds
MAX_WAIT_TIME = int(os.getenv("MAX_WAIT_TIME", 5000))  # milliseconds
