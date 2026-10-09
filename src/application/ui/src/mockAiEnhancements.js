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

// Mock data for AI-enhanced content
const mockAiEnhancements = {
    // Enhanced product images
    images: {
        'default': 'https://m.media-amazon.com/images/I/61ahwlqvxwL._AC_SL1500_.jpg', // Higher quality version
    },

    // Enhanced product descriptions
    descriptions: {
        'default': `Experience the vibrant and refreshing taste of San Pellegrino Aranciata, a premium Italian sparkling beverage crafted with care since 1932. Made with hand-picked oranges from sun-drenched Mediterranean groves, this exquisite drink combines the perfect balance of sweetness and citrus zest with delicate carbonation.

Each elegant aluminum can preserves the authentic flavor profile that has made San Pellegrino a globally celebrated name in fine beverages. The unique recipe features real orange juice and essential oils from the peel, creating a sophisticated flavor that awakens the senses with every sip.

Perfect for enjoying on its own over ice, paired with fine cuisine, or as a sophisticated mixer for premium cocktails. San Pellegrino Aranciata embodies Italian craftsmanship and the Mediterranean lifestyle - a moment of everyday luxury that transports you to sunny Italian terraces.`,
    },

    // Enhanced product specifications
    specifications: {
        'default': [
            { name: "Volume", value: "330ml", isNew: false },
            { name: "Container", value: "Aluminum Can", isNew: false },
            { name: "Country of Origin", value: "Italy", isNew: false },
            { name: "Calories", value: "90 per can", isNew: true },
            { name: "Ingredients", value: "Carbonated Water, Orange Juice (16%), Sugar, Orange Extract, Natural Flavors", isNew: true },
            { name: "Carbonation Level", value: "Medium", isNew: true },
            { name: "Serving Temperature", value: "Cold (4-6°C / 39-43°F)", isNew: true },
            { name: "Shelf Life", value: "18 months unopened", isNew: true },
            { name: "Packaging", value: "Recyclable aluminum can", isNew: false, changed: true },
            { name: "Storage Instructions", value: "Store in a cool, dry place", isNew: true },
        ]
    }
};

export default mockAiEnhancements;
