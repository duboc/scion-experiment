/*
 * Copyright 2025 Google LLC
 * 
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 * 
 *     https://www.apache.org/licenses/LICENSE-2.0
 * 
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

export const mockProducts = [
  {
    id: "prod_001",
    name: "Classic Cotton T-Shirt",
    title: "Premium Cotton Crew Neck T-Shirt",
    brands: ["EssentialWear"],
    categories: ["Clothing", "T-Shirts", "Basics"],
    priceInfo: {
      cost: 15.00,
      currencyCode: "USD",
      originalPrice: 24.99,
      price: 19.99,
      priceEffectiveTime: "2024-02-01T00:00:00Z",
      priceExpireTime: "2024-12-31T23:59:59Z"
    },
    colorInfo: {
      colorFamilies: ["Neutral", "Basic"],
      colors: ["White", "Black", "Gray"]
    },
    availability: "IN_STOCK",
    availableQuantity: 100,
    availableTime: "2024-02-01T00:00:00Z",
    images: [
      {
        height: 800,
        width: 600,
        uri: "https://images.unsplash.com/photo-1521572163474-6864f9cf17ab?w=800&h=600&fit=crop"
      }
    ],
    sizes: ["XS", "S", "M", "L", "XL"],
    uri: "/products/classic-cotton-tshirt"
  },
  {
    id: "prod_002",
    name: "Premium Denim Jeans",
    title: "Slim Fit Stretch Denim Jeans",
    brands: ["DenimCo"],
    categories: ["Clothing", "Jeans", "Premium"],
    priceInfo: {
      cost: 45.00,
      currencyCode: "USD",
      originalPrice: 89.99,
      price: 79.99,
      priceEffectiveTime: "2024-02-01T00:00:00Z",
      priceExpireTime: "2024-12-31T23:59:59Z"
    },
    colorInfo: {
      colorFamilies: ["Blue"],
      colors: ["Light Wash", "Dark Wash", "Medium Wash"]
    },
    availability: "IN_STOCK",
    availableQuantity: 50,
    availableTime: "2024-02-01T00:00:00Z",
    images: [
      {
        height: 800,
        width: 600,
        uri: "https://images.unsplash.com/photo-1542272604-787c3835535d?w=800&h=600&fit=crop"
      }
    ],
    sizes: ["28x30", "30x32", "32x32", "34x34"],
    uri: "/products/premium-denim-jeans"
  },
  {
    id: "prod_003",
    name: "Running Shoes Pro",
    title: "Professional Running Shoes with Air Cushion",
    brands: ["SportFit"],
    categories: ["Shoes", "Athletic", "Running"],
    priceInfo: {
      cost: 75.00,
      currencyCode: "USD",
      originalPrice: 149.99,
      price: 129.99,
      priceEffectiveTime: "2024-02-01T00:00:00Z",
      priceExpireTime: "2024-12-31T23:59:59Z"
    },
    colorInfo: {
      colorFamilies: ["Athletic"],
      colors: ["Black/Red", "White/Blue", "Gray/Yellow"]
    },
    availability: "IN_STOCK",
    availableQuantity: 25,
    availableTime: "2024-02-01T00:00:00Z",
    images: [
      {
        height: 800,
        width: 600,
        uri: "https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800&h=600&fit=crop"
      }
    ],
    sizes: ["7", "8", "9", "10", "11", "12"],
    uri: "/products/running-shoes-pro"
  },
  {
    id: "prod_004",
    name: "Smart Watch Elite",
    title: "Advanced Fitness Tracking Smartwatch",
    brands: ["TechGear"],
    categories: ["Electronics", "Wearables", "Smart Watches"],
    priceInfo: {
      cost: 120.00,
      currencyCode: "USD",
      originalPrice: 199.99,
      price: 179.99,
      priceEffectiveTime: "2024-02-01T00:00:00Z",
      priceExpireTime: "2024-12-31T23:59:59Z"
    },
    colorInfo: {
      colorFamilies: ["Modern"],
      colors: ["Space Gray", "Silver", "Rose Gold"]
    },
    availability: "IN_STOCK",
    availableQuantity: 15,
    availableTime: "2024-02-01T00:00:00Z",
    images: [
      {
        height: 800,
        width: 600,
        uri: "https://images.unsplash.com/photo-1546868871-7041f2a55e12?w=800&h=600&fit=crop"
      }
    ],
    sizes: ["One Size"],
    uri: "/products/smart-watch-elite"
  },
  {
    id: "prod_005",
    name: "Leather Backpack",
    title: "Vintage Style Leather Travel Backpack",
    brands: ["Urban Carry"],
    categories: ["Accessories", "Bags", "Travel"],
    priceInfo: {
      cost: 85.00,
      currencyCode: "USD",
      originalPrice: 159.99,
      price: 139.99,
      priceEffectiveTime: "2024-02-01T00:00:00Z",
      priceExpireTime: "2024-12-31T23:59:59Z"
    },
    colorInfo: {
      colorFamilies: ["Brown", "Classic"],
      colors: ["Tan", "Dark Brown", "Black"]
    },
    availability: "OUT_OF_STOCK",
    availableQuantity: 0,
    availableTime: "2024-03-15T00:00:00Z",
    images: [
      {
        height: 800,
        width: 600,
        uri: "https://images.unsplash.com/photo-1553062407-98eeb64c6a62?w=800&h=600&fit=crop"
      }
    ],
    sizes: ["Standard"],
    uri: "/products/leather-backpack"
  },
  {
    id: "prod_006",
    name: "Wireless Headphones",
    title: "Premium Noise Cancelling Bluetooth Headphones",
    brands: ["AudioPro"],
    categories: ["Electronics", "Audio", "Headphones"],
    priceInfo: {
      cost: 150.00,
      currencyCode: "USD",
      originalPrice: 299.99,
      price: 249.99,
      priceEffectiveTime: "2024-02-01T00:00:00Z",
      priceExpireTime: "2024-12-31T23:59:59Z"
    },
    colorInfo: {
      colorFamilies: ["Modern"],
      colors: ["Matte Black", "White", "Navy Blue"]
    },
    availability: "IN_STOCK",
    availableQuantity: 30,
    availableTime: "2024-02-01T00:00:00Z",
    images: [
      {
        height: 800,
        width: 600,
        uri: "https://images.unsplash.com/photo-1505740420928-5e560c06d30e?w=800&h=600&fit=crop"
      }
    ],
    sizes: ["One Size"],
    uri: "/products/wireless-headphones"
  }
]; 