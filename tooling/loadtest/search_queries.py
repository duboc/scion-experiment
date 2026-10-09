"""
Sample search queries for load testing the PSearch API.
These queries cover various clothing and accessory categories from famous brands,
and parameter combinations.

Parameter validation rules:
- query: Must be a non-empty string
- limit: Must be a positive integer (default: 10)
- min_score: Must be a number between 0 and 1 (default: 0.0)
- alpha: Must be a number between 0.0 and 1.0 (default: 0.5)
"""

SEARCH_QUERIES = [
    # Basic queries - Clothing & Accessories with Famous Brands
    {"query": "Nike Air Max sneakers"},
    {"query": "Gucci handbag"},
    {"query": "Levi's denim jacket"},
    {"query": "Ray-Ban sunglasses"},
    {"query": "Adidas track pants"},
    
    # Queries with filters - Clothing & Accessories with Famous Brands
    {"query": "Prada dresses", "limit": 5},
    {"query": "Chanel perfume", "min_score": 0.7},
    {"query": "Burberry trench coat", "alpha": 0.8},
    {"query": "Lululemon leggings", "limit": 15},
    {"query": "Michael Kors tote bag", "min_score": 0.3},
    
    # Complex queries with all parameters - Clothing & Accessories with Famous Brands
    {"query": "Ralph Lauren polo shirt", "limit": 20, "min_score": 0.6, "alpha": 0.3},
    {"query": "Versace silk scarf", "limit": 10, "min_score": 0.5, "alpha": 0.7},
    {"query": "Under Armour compression shorts", "limit": 8, "min_score": 0.4, "alpha": 0.6},
    {"query": "Calvin Klein men's underwear", "limit": 25, "min_score": 0.2, "alpha": 0.5},
    
    # Specific product types - Clothing & Accessories with Famous Brands
    {"query": "Nike Air Jordan 1 men's size 11"},
    {"query": "Zara women's black blazer size S"},
    {"query": "Gucci Dionysus shoulder bag small"},
    {"query": "Rolex Submariner steel black dial"},
    {"query": "Hermès Birkin bag togo leather"},
]
