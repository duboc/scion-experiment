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

import React, { useState, useMemo, useCallback, useEffect, useRef, memo } from 'react';
import { BrowserRouter as Router, Routes, Route, Link, useNavigate } from 'react-router-dom';
import {
    Container,
    Grid,
    TextField,
    Card,
    CardContent,
    Typography,
    Box,
    Chip,
    AppBar,
    Toolbar,
    Button,
    FormControl,
    InputLabel,
    Select,
    MenuItem,
    Alert,
    ThemeProvider,
    createTheme,
} from '@mui/material';
import SearchIcon from '@mui/icons-material/Search';
import SettingsIcon from '@mui/icons-material/Settings';
import FileUploadIcon from '@mui/icons-material/FileUpload';
import CampaignIcon from '@mui/icons-material/Campaign';
import RuleManager from './components/RuleManager';
import AdCampaignManager from './components/AdCampaignManager';
import ProductDetails from './components/ProductDetails';
import SourceIngestion from './components/SourceIngestion';
import Filters from './components/Filters'; // Import the custom filters component
import AIFilterSuggestion from './components/AIFilterSuggestion'; // Import the AI filter suggestion component
import ProductImage from './components/ProductImage'; // Import the optimized ProductImage component
import SearchInput from './components/SearchInput'; // Import the new isolated search component
import SessionPanel from './components/SessionPanel';
import { getSessionId, recordSessionEvent } from './services/sessionService';
import { recordAdClick } from './services/adService';
import axios from 'axios';
import config from './config';
const API_URL = config.apiUrl;

const COLOR_SWATCH_MAP = {
    preto: '#1e293b',
    black: '#1e293b',
    branco: '#f8fafc',
    white: '#f8fafc',
    azul: '#2563eb',
    blue: '#2563eb',
    prata: '#94a3b8',
    silver: '#94a3b8',
    grafite: '#475569',
    graphite: '#475569',
};

function ProductSearch({
    tenantId: propTenantId,
    setTenantId: propSetTenantId,
    locale: propLocale,
    setLocale: propSetLocale,
}) {
    const navigate = useNavigate();
    // Simplified search state - only track the active search query
    const [activeSearchQuery, setActiveSearchQuery] = useState(''); // For actual API requests
    const [showAIFilterSuggestions, setShowAIFilterSuggestions] = useState(false); // Control visibility of AI filter suggestions
    const [products, setProducts] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const [alpha, setAlpha] = useState(0.5);
    const [channel, setChannel] = useState('');
    const [onlyInStock, setOnlyInStock] = useState(false);
    const [cep, setCep] = useState('');
    const [maxSla24h, setMaxSla24h] = useState(false);
    const [ltr, setLtr] = useState(false);
    const [effectiveLtr, setEffectiveLtr] = useState(false);
    const [ltrModel, setLtrModel] = useState(null);
    const [ltrLatencyUs, setLtrLatencyUs] = useState(null);
    const [parseIntent, setParseIntent] = useState(true);
    const [parsedQuery, setParsedQuery] = useState(null);
    const [activeExtractedFilters, setActiveExtractedFilters] = useState(null);
    const [cleanQueryBase, setCleanQueryBase] = useState('');
    const [autoCorrected, setAutoCorrected] = useState(false);
    const [queryRelaxed, setQueryRelaxed] = useState(false);
    const [didYouMean, setDidYouMean] = useState('');
    const [responseQuery, setResponseQuery] = useState('');
    const activeExtractedFiltersRef = useRef(activeExtractedFilters);
    activeExtractedFiltersRef.current = activeExtractedFilters;
    const parseIntentRef = useRef(parseIntent);
    parseIntentRef.current = parseIntent;
    const cleanQueryBaseRef = useRef(cleanQueryBase);
    cleanQueryBaseRef.current = cleanQueryBase;
    const [sessionId, setSessionId] = useState(() => getSessionId());
    const sessionIdRef = useRef(sessionId);
    sessionIdRef.current = sessionId;
    const [personalized, setPersonalized] = useState(false);
    const [sessionRefreshKey, setSessionRefreshKey] = useState(0);
    const [internalTenantId, setInternalTenantId] = useState('default');
    const [internalLocale, setInternalLocale] = useState('');
    const tenantId = propTenantId !== undefined ? propTenantId : internalTenantId;
    const setTenantId = propSetTenantId || setInternalTenantId;
    const locale = propLocale !== undefined ? propLocale : internalLocale;
    const setLocale = propSetLocale || setInternalLocale;
    const [effectiveAlpha, setEffectiveAlpha] = useState(null);
    const [retrievalMode, setRetrievalMode] = useState(null);
    const [effectiveTenantId, setEffectiveTenantId] = useState(null);
    const [effectiveLocale, setEffectiveLocale] = useState(null);
    const [expandedTerms, setExpandedTerms] = useState([]);
    const [appliedRules, setAppliedRules] = useState([]);
    const [priceTableId, setPriceTableId] = useState('');
    const [effectivePriceTableId, setEffectivePriceTableId] = useState('');
    const [sortBy, setSortBy] = useState('relevance');
    const [effectiveSortBy, setEffectiveSortBy] = useState('');
    const [color, setColor] = useState('');
    const [size, setSize] = useState('');
    const [serverFacets, setServerFacets] = useState(null);
    const [facetsLatencyMs, setFacetsLatencyMs] = useState(null);
    const [sponsoredCount, setSponsoredCount] = useState(0);
    const [vectorIndexUsed, setVectorIndexUsed] = useState(null);
    // State for the filters component - now a dynamic object
    const [selectedFilters, setSelectedFilters] = useState({});
    // Store filter configurations
    const [filterConfigs, setFilterConfigs] = useState([]);
    const [currentPage, setCurrentPage] = useState(1);
    const [productsPerPage, setProductsPerPage] = useState(24);
    const [totalFound, setTotalFound] = useState(0);
    const [serverTotalPages, setServerTotalPages] = useState(0);
    const [serverHasNext, setServerHasNext] = useState(false);
    const [serverOffset, setServerOffset] = useState(0);

    // Format a filter name for display (convert lens_color to Lens Color)
    const formatFilterName = (name) => {
        return name
            .split('_')
            .map(word => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
            .join(' ');
    };

    // Generate filter configurations based on product data (excluding sponsored items per ADR-011)
    const generateFilterConfigs = useCallback((rawProducts) => {
        const products = Array.isArray(rawProducts)
            ? rawProducts.filter(product => !product || !product.sponsored)
            : [];
        if (products.length === 0) {
            return [];
        }

        const configs = [];
        // Track filter IDs to prevent duplicates
        const addedFilterIds = new Set();

        const filterAdders = [
            // Categories filter
            () => {
                const categoryCounts = new Map();
                products.forEach(product => {
                    if (product.categories) {
                        product.categories.forEach(category => {
                            categoryCounts.set(category, (categoryCounts.get(category) || 0) + 1);
                        });
                    }
                });

                const categories = Array.from(categoryCounts.keys());
                if (categories.length > 0) {
                    configs.push({
                        id: 'categories',
                        title: 'Department',
                        priority: 100, // High priority
                        options: categories.map(cat => ({ label: cat, value: cat, count: categoryCounts.get(cat) })),
                        type: 'checkbox'
                    });
                }
            },

            // Brands filter
            () => {
                const brandCounts = new Map();
                products.forEach(product => {
                    if (product.brands) {
                        product.brands.forEach(brand => {
                            brandCounts.set(brand, (brandCounts.get(brand) || 0) + 1);
                        });
                    }
                    // Also check attributes for brand if top-level is missing
                    if (!product.brands && product.attributes) {
                        const brandAttr = product.attributes.find(attr => attr.key === 'brand');
                        if (brandAttr && brandAttr.value?.text) {
                            brandAttr.value.text.forEach(b => {
                                brandCounts.set(b, (brandCounts.get(b) || 0) + 1);
                            });
                        }
                    }
                });

                const brands = Array.from(brandCounts.keys());
                if (brands.length > 0) {
                    configs.push({
                        id: 'brands',
                        title: 'Brands',
                        priority: 90, // High priority
                        options: brands.map(brand => ({ label: brand, value: brand, count: brandCounts.get(brand) })),
                        type: 'checkbox'
                    });
                }
            },

            // Price ranges filter
            () => {
                const validPrices = products
                    .map(p => p.priceInfo?.price)
                    .filter(price => price !== null && price !== undefined && !isNaN(parseFloat(price)))
                    .map(price => parseFloat(price));

                if (validPrices.length < 2) return; // Need at least two prices to make ranges

                const minPrice = Math.min(...validPrices);
                const maxPrice = Math.max(...validPrices);

                if (minPrice === maxPrice) {
                    configs.push({
                        id: 'prices',
                        title: 'Price',
                        priority: 80, // High priority
                        options: [{ label: `$${minPrice.toFixed(2)}`, value: `${minPrice}-${minPrice}`, count: validPrices.length }],
                        type: 'checkbox'
                    });
                    return;
                }

                const numRanges = 4; // Define number of ranges
                const step = (maxPrice - minPrice) / numRanges;
                const ranges = [];

                for (let i = 0; i < numRanges; i++) {
                    const rangeMin = minPrice + i * step;
                    const rangeMax = minPrice + (i + 1) * step;
                    // Ensure the last range includes the max price
                    const finalMax = (i === numRanges - 1) ? maxPrice : rangeMax;
                    const fMin = Math.floor(rangeMin);
                    const fMax = Math.ceil(finalMax);
                    const rangeCount = validPrices.filter(p => p >= fMin && p <= fMax).length;

                    ranges.push({
                        // Use Math.floor/ceil to avoid overlapping ranges due to floating point issues
                        label: `$${fMin.toFixed(2)} - $${fMax.toFixed(2)}`,
                        value: `${fMin}-${fMax}`,
                        count: rangeCount
                    });
                }

                configs.push({
                    id: 'prices',
                    title: 'Price',
                    options: ranges,
                    type: 'checkbox'
                });
            },

            // Colors filter
            () => {
                const colorCounts = new Map();
                products.forEach(product => {
                    const prodColors = new Set();
                    if (product.colorInfo?.colors) {
                        product.colorInfo.colors.forEach(color => prodColors.add(color));
                    }
                    if (product.colorInfo?.colorFamilies) {
                        product.colorInfo.colorFamilies.forEach(color => prodColors.add(color));
                    }
                    if (Array.isArray(product.available_colors)) {
                        product.available_colors.forEach(color => prodColors.add(color));
                    }
                    prodColors.forEach(color => {
                        colorCounts.set(color, (colorCounts.get(color) || 0) + 1);
                    });
                });

                const colors = Array.from(colorCounts.keys());
                if (colors.length > 0) {
                    configs.push({
                        id: 'colors',
                        title: 'Cor',
                        priority: 70, // Medium-high priority
                        options: colors.map(color => ({ label: color, value: color, count: colorCounts.get(color) })),
                        type: 'checkbox'
                    });
                }
            },

            // Sizes filter
            () => {
                const sizeCounts = new Map();
                products.forEach(product => {
                    const prodSizes = new Set();
                    if (product.sizes) {
                        product.sizes.forEach(size => prodSizes.add(size));
                    }
                    if (Array.isArray(product.available_sizes)) {
                        product.available_sizes.forEach(size => prodSizes.add(size));
                    }
                    prodSizes.forEach(size => {
                        sizeCounts.set(size, (sizeCounts.get(size) || 0) + 1);
                    });
                });

                const sizes = Array.from(sizeCounts.keys());
                if (sizes.length > 0) {
                    configs.push({
                        id: 'sizes',
                        title: 'Tamanho',
                        priority: 60, // Medium priority
                        options: sizes.map(size => ({ label: size, value: size, count: sizeCounts.get(size) })),
                        type: 'checkbox'
                    });
                }
            },

            // Availability filter
            () => {
                const inStockCount = products.filter(p => p.availability === 'IN_STOCK').length;
                const outOfStockCount = products.filter(p => p.availability === 'OUT_OF_STOCK').length;

                if (inStockCount > 0 && outOfStockCount > 0) {
                    configs.push({
                        id: 'availability',
                        title: 'Availability',
                        priority: 50, // Medium priority
                        options: [
                            { label: 'In Stock', value: 'IN_STOCK', count: inStockCount },
                            { label: 'Out of Stock', value: 'OUT_OF_STOCK', count: outOfStockCount }
                        ],
                        type: 'checkbox'
                    });
                }
            },

            // Dynamic attributes filter
            () => {
                // Find common attribute keys that appear in multiple products
                const attributeKeys = new Map();

                products.forEach(product => {
                    if (product.attributes) {
                        product.attributes.forEach(attr => {
                            if (attr.key && attr.value) {
                                const count = attributeKeys.get(attr.key) || 0;
                                attributeKeys.set(attr.key, count + 1);
                            }
                        });
                    }
                });

                // For each common attribute, create a filter
                attributeKeys.forEach((count, key) => {
                    // Skip if it only appears in one product or is already used (like brand)
                    if (count < 2 || key === 'brand') return;

                    // Get all values and counts for this attribute
                    const valCounts = new Map();
                    products.forEach(product => {
                        if (product.attributes) {
                            const attr = product.attributes.find(a => a.key === key);
                            if (attr && attr.value?.text) {
                                attr.value.text.forEach(val => {
                                    valCounts.set(val, (valCounts.get(val) || 0) + 1);
                                });
                            }
                        }
                    });

                    if (valCounts.size > 0) {
                        configs.push({
                            id: `attr_${key}`,
                            title: formatFilterName(key),
                            priority: 40, // Lower priority for dynamic attributes
                            options: Array.from(valCounts.keys()).map(value => ({ label: value, value, count: valCounts.get(value) })),
                            type: 'checkbox'
                        });
                    }
                });
            }
        ];

        // Apply all filter adders
        filterAdders.forEach(adder => adder());

        // Remove any duplicate filters by ID 
        let uniqueConfigs = configs.filter(config => {
            if (addedFilterIds.has(config.id)) {
                return false;
            }
            addedFilterIds.add(config.id);
            return true;
        });

        // Prepare the final filter list
        const finalFilters = [];

        // 1. Ensure Department filter is included
        // Department is typically derived from categories
        const categoryFilter = uniqueConfigs.find(config => config.id === 'categories');
        if (categoryFilter) {
            // Rename to Department for better user understanding
            categoryFilter.title = 'Department';
            finalFilters.push(categoryFilter);
            // Remove from uniqueConfigs to avoid duplication
            uniqueConfigs = uniqueConfigs.filter(config => config.id !== 'categories');
        }

        // 2. First ensure Price filter is included (if available)
        const priceFilter = uniqueConfigs.find(config => config.id === 'prices');
        if (priceFilter) {
            finalFilters.push(priceFilter);
            // Remove from uniqueConfigs to avoid duplication
            uniqueConfigs = uniqueConfigs.filter(config => config.id !== 'prices');
        }

        // 3. Sort remaining filters by priority and add up
        const remainingSlots = 10 - finalFilters.length;
        if (remainingSlots > 0 && uniqueConfigs.length > 0) {
            const remainingFilters = uniqueConfigs
                .sort((a, b) => (b.priority || 0) - (a.priority || 0))
                .slice(0, remainingSlots);

            finalFilters.push(...remainingFilters);
        }

        return finalFilters;
    }, []);

    // Generate filter configurations from server-side facets (#41, ADR-013)
    const generateFilterConfigsFromFacets = useCallback((facets, currentSelected = {}) => {
        if (!facets || typeof facets !== 'object') return [];

        const mergeSelectedOptions = (options, selectedList, formatLabel) => {
            const list = Array.isArray(options) ? [...options] : [];
            const sel = Array.isArray(selectedList) ? selectedList : [];
            sel.forEach(val => {
                if (!val) return;
                const exists = list.some(o => String(o.value).toLowerCase() === String(val).toLowerCase());
                if (!exists) {
                    list.push({
                        label: formatLabel ? formatLabel(val) : val,
                        value: val,
                        count: 0,
                        selected: true,
                    });
                }
            });
            return list;
        };

        const formatPriceLabel = (val) => {
            if (typeof val === 'string') {
                if (val.endsWith('+')) {
                    return `$${val.slice(0, -1)}+`;
                }
                if (val.includes('-')) {
                    const parts = val.split('-');
                    return `$${parts[0]} - $${parts[1]}`;
                }
            }
            return val;
        };

        const configs = [];

        const catOptions = mergeSelectedOptions(
            (facets.categories || []).map(c => ({
                label: typeof c === 'object' ? c.value : c,
                value: typeof c === 'object' ? c.value : c,
                count: typeof c === 'object' ? c.count : undefined,
                selected: typeof c === 'object' ? Boolean(c.selected) : false,
            })),
            currentSelected.categories
        );
        if (catOptions.length > 0) {
            configs.push({
                id: 'categories',
                title: 'Department',
                priority: 100,
                type: 'checkbox',
                options: catOptions,
            });
        }

        const brandOptions = mergeSelectedOptions(
            (facets.brands || []).map(b => ({
                label: typeof b === 'object' ? b.value : b,
                value: typeof b === 'object' ? b.value : b,
                count: typeof b === 'object' ? b.count : undefined,
                selected: typeof b === 'object' ? Boolean(b.selected) : false,
            })),
            currentSelected.brands
        );
        if (brandOptions.length > 0) {
            configs.push({
                id: 'brands',
                title: 'Brands',
                priority: 90,
                type: 'checkbox',
                options: brandOptions,
            });
        }

        const priceOptions = mergeSelectedOptions(
            (facets.price_ranges || []).map(p => {
                const val = typeof p === 'object' ? p.value : p;
                const count = typeof p === 'object' ? p.count : undefined;
                return {
                    label: formatPriceLabel(val),
                    value: val,
                    count,
                    selected: typeof p === 'object' ? Boolean(p.selected) : false,
                };
            }),
            currentSelected.prices,
            formatPriceLabel
        );
        if (priceOptions.length > 0) {
            configs.push({
                id: 'prices',
                title: 'Price',
                priority: 80,
                type: 'checkbox',
                options: priceOptions,
            });
        }

        const colorOptions = mergeSelectedOptions(
            (facets.colors || []).map(c => ({
                label: typeof c === 'object' ? c.value : c,
                value: typeof c === 'object' ? c.value : c,
                count: typeof c === 'object' ? c.count : undefined,
                selected: typeof c === 'object' ? Boolean(c.selected) : false,
            })),
            currentSelected.colors
        );
        if (colorOptions.length > 0) {
            configs.push({
                id: 'colors',
                title: 'Cor',
                priority: 75,
                type: 'checkbox',
                options: colorOptions,
            });
        }

        const sizeOptions = mergeSelectedOptions(
            (facets.sizes || []).map(s => ({
                label: typeof s === 'object' ? s.value : s,
                value: typeof s === 'object' ? s.value : s,
                count: typeof s === 'object' ? s.count : undefined,
                selected: typeof s === 'object' ? Boolean(s.selected) : false,
            })),
            currentSelected.sizes
        );
        if (sizeOptions.length > 0) {
            configs.push({
                id: 'sizes',
                title: 'Tamanho',
                priority: 72,
                type: 'checkbox',
                options: sizeOptions,
            });
        }

        if (Array.isArray(facets.availability) && facets.availability.length > 0) {
            configs.push({
                id: 'availability',
                title: 'Availability',
                priority: 70,
                type: 'checkbox',
                options: facets.availability.map(a => {
                    const val = typeof a === 'object' ? a.value : a;
                    const count = typeof a === 'object' ? a.count : undefined;
                    let label = val;
                    let optVal = val;
                    if (val === 'IN_STOCK' || val === true || val === 'true') {
                        label = 'In Stock';
                        optVal = 'IN_STOCK';
                    } else if (val === 'OUT_OF_STOCK' || val === false || val === 'false') {
                        label = 'Out of Stock';
                        optVal = 'OUT_OF_STOCK';
                    }
                    return {
                        label,
                        value: optVal,
                        count,
                    };
                })
            });
        }

        return configs;
    }, []);

    // Use memoized filter configurations: prioritize serverFacets, fall back to generateFilterConfigs
    useEffect(() => {
        if (serverFacets) {
            const facetConfigs = generateFilterConfigsFromFacets(serverFacets, selectedFilters);
            if (facetConfigs.length > 0) {
                setFilterConfigs(facetConfigs);
                return;
            }
        }
        const configs = generateFilterConfigs(products);
        setFilterConfigs(configs);
    }, [products, serverFacets, selectedFilters, generateFilterConfigs, generateFilterConfigsFromFacets]);

    // The ProductImage component has been moved to its own file for better performance

    const selectedCategories = useMemo(
        () => (Array.isArray(selectedFilters.categories) ? selectedFilters.categories.filter(Boolean) : []),
        [selectedFilters.categories]
    );
    const selectedBrands = useMemo(
        () => (Array.isArray(selectedFilters.brands) ? selectedFilters.brands.filter(Boolean) : []),
        [selectedFilters.brands]
    );
    const selectedColors = useMemo(() => {
        const set = new Set(Array.isArray(selectedFilters.colors) ? selectedFilters.colors.filter(Boolean) : []);
        if (color) set.add(color);
        return Array.from(set);
    }, [selectedFilters.colors, color]);
    const selectedSizes = useMemo(() => {
        const set = new Set(Array.isArray(selectedFilters.sizes) ? selectedFilters.sizes.filter(Boolean) : []);
        if (size) set.add(size);
        return Array.from(set);
    }, [selectedFilters.sizes, size]);
    const selectedPriceRanges = useMemo(
        () => (Array.isArray(selectedFilters.prices) ? selectedFilters.prices.filter(Boolean) : []),
        [selectedFilters.prices]
    );
    const selectedAvailability = useMemo(
        () => (Array.isArray(selectedFilters.availability) ? selectedFilters.availability.filter(Boolean) : []),
        [selectedFilters.availability]
    );

    const queryImageRef = useRef(null); // #33: current photo query (data URL) or null
    const fetchProducts = useCallback(async (searchParams) => {
        setLoading(true);
        setError(null);
        try {
            const queryToUse = (searchParams && searchParams.query !== undefined)
                ? searchParams.query
                : activeSearchQuery;
            const parseIntentToUse = (searchParams && searchParams.parseIntent !== undefined)
                ? searchParams.parseIntent
                : parseIntentRef.current;
            const extractedFiltersToUse = (searchParams && searchParams.extractedFilters !== undefined)
                ? searchParams.extractedFilters
                : activeExtractedFiltersRef.current;
            const priceTableIdToUse = (searchParams && searchParams.priceTableId !== undefined)
                ? searchParams.priceTableId
                : priceTableId;
            const sortByToUse = (searchParams && searchParams.sortBy !== undefined)
                ? searchParams.sortBy
                : sortBy;
            const colorsToUse = (searchParams && searchParams.color !== undefined)
                ? (searchParams.color ? [searchParams.color] : [])
                : selectedColors;
            const sizesToUse = (searchParams && searchParams.size !== undefined)
                ? (searchParams.size ? [searchParams.size] : [])
                : selectedSizes;
            const imageToUse = (searchParams && searchParams.image !== undefined)
                ? searchParams.image
                : queryImageRef.current;
            const sessionIdToUse = (searchParams && searchParams.sessionId !== undefined)
                ? searchParams.sessionId
                : (sessionIdRef.current || getSessionId());
            const pageToUse = (searchParams && searchParams.page !== undefined)
                ? Math.max(1, Number(searchParams.page) || 1)
                : 1;
            const pageSizeToUse = (searchParams && searchParams.pageSize !== undefined)
                ? (Number(searchParams.pageSize) || 24)
                : productsPerPage;
            const offsetToUse = (pageToUse - 1) * pageSizeToUse;

            const effectiveOnlyInStock = Boolean(
                onlyInStock ||
                (selectedAvailability.includes('IN_STOCK') && !selectedAvailability.includes('OUT_OF_STOCK'))
            );

            const payload = {
                query: queryToUse,
                limit: pageSizeToUse,
                page: pageToUse,
                offset: offsetToUse,
                min_score: 0.01,
                alpha: typeof alpha === 'number' ? alpha : 0.5,
                only_in_stock: effectiveOnlyInStock,
                tenant_id: tenantId || 'default',
            };
            if (sessionIdToUse) {
                payload.session_id = sessionIdToUse;
            }
            if (parseIntentToUse !== undefined) {
                payload.parse_intent = Boolean(parseIntentToUse);
            }
            if (extractedFiltersToUse) {
                if (extractedFiltersToUse.min_price !== undefined && extractedFiltersToUse.min_price !== null && Number(extractedFiltersToUse.min_price) > 0) {
                    payload.min_price = Number(extractedFiltersToUse.min_price);
                }
                if (extractedFiltersToUse.max_price !== undefined && extractedFiltersToUse.max_price !== null && Number(extractedFiltersToUse.max_price) > 0) {
                    payload.max_price = Number(extractedFiltersToUse.max_price);
                }
                if (extractedFiltersToUse.brand) {
                    payload.brand = String(extractedFiltersToUse.brand);
                }
                if (extractedFiltersToUse.category) {
                    payload.category = String(extractedFiltersToUse.category);
                }
                if (extractedFiltersToUse.color) {
                    payload.color = String(extractedFiltersToUse.color);
                }
            }
            if (locale) {
                payload.locale = locale;
            }
            if (channel && channel !== 'All' && channel !== 'all') {
                payload.channel = channel;
            }
            if (selectedCategories.length > 0 && (!extractedFiltersToUse || !extractedFiltersToUse.category)) {
                payload.categories = selectedCategories;
                if (selectedCategories.length === 1) {
                    payload.category = selectedCategories[0];
                }
            }
            if (selectedBrands.length > 0 && (!extractedFiltersToUse || !extractedFiltersToUse.brand)) {
                payload.brands = selectedBrands;
                if (selectedBrands.length === 1) {
                    payload.brand = selectedBrands[0];
                }
            }
            if (colorsToUse.length > 0) {
                payload.colors = colorsToUse;
                if (colorsToUse.length === 1) {
                    payload.color = colorsToUse[0];
                }
            }
            if (sizesToUse.length > 0) {
                payload.sizes = sizesToUse;
                if (sizesToUse.length === 1) {
                    payload.size = sizesToUse[0];
                }
            }
            if (selectedPriceRanges.length > 0) {
                payload.price_ranges = selectedPriceRanges;
            }
            if (cep && cep.trim()) {
                payload.cep = cep.trim();
            }
            if (maxSla24h) {
                payload.max_sla_hours = 24;
            }
            if (ltr) {
                payload.ltr = true;
            }
            if (priceTableIdToUse) {
                payload.price_table_id = priceTableIdToUse;
            }
            if (sortByToUse) {
                payload.sort_by = sortByToUse;
            }

            if (imageToUse) {
                // Visual search (#33, ADR-010): inline photo; omit alpha so the
                // server weights the vector (photo) leg at its multimodal default.
                payload.image_base64 = imageToUse;
                payload.image_mime_type = 'image/jpeg';
                delete payload.alpha;
            }

            const response = await axios.post(`${API_URL}/search`, payload);

            console.log("API Response:", JSON.stringify(response.data, null, 2));

            if (response.data) {
                setResponseQuery(response.data.query || queryToUse);
                setAutoCorrected(Boolean(response.data.auto_corrected));
                setDidYouMean(response.data.did_you_mean || '');
                setQueryRelaxed(Boolean(response.data.query_relaxed));
                if (response.data.parsed_query) {
                    setParsedQuery(response.data.parsed_query);
                    const ext = response.data.parsed_query.extracted_filters || {};
                    setActiveExtractedFilters(ext);
                    activeExtractedFiltersRef.current = ext;
                    const cleanQ = response.data.parsed_query.clean_query || queryToUse;
                    setCleanQueryBase(cleanQ);
                    cleanQueryBaseRef.current = cleanQ;
                } else if (searchParams && searchParams.extractedFilters !== undefined) {
                    setActiveExtractedFilters(searchParams.extractedFilters);
                    activeExtractedFiltersRef.current = searchParams.extractedFilters;
                } else if (!parseIntentToUse) {
                    setParsedQuery(null);
                }
                if (response.data.effective_alpha !== undefined) {
                    setEffectiveAlpha(response.data.effective_alpha);
                }
                if (response.data.retrieval_mode !== undefined) {
                    setRetrievalMode(response.data.retrieval_mode);
                }
                if (response.data.tenant_id !== undefined) {
                    setEffectiveTenantId(response.data.tenant_id);
                }
                if (response.data.locale !== undefined) {
                    setEffectiveLocale(response.data.locale);
                }
                if (response.data.expanded_terms && Array.isArray(response.data.expanded_terms)) {
                    setExpandedTerms(response.data.expanded_terms);
                } else {
                    setExpandedTerms([]);
                }
                if (response.data.applied_rules && Array.isArray(response.data.applied_rules)) {
                    setAppliedRules(response.data.applied_rules);
                } else {
                    setAppliedRules([]);
                }
                if (response.data.facets && Object.keys(response.data.facets).length > 0) {
                    setServerFacets(response.data.facets);
                } else {
                    setServerFacets(null);
                }
                const fLat = response.data.facets_latency_ms !== undefined
                    ? response.data.facets_latency_ms
                    : (response.data.facets && response.data.facets.latency_ms);
                if (fLat !== undefined && fLat !== null) {
                    setFacetsLatencyMs(fLat);
                } else {
                    setFacetsLatencyMs(null);
                }
                if (response.data.ltr !== undefined) {
                    setEffectiveLtr(Boolean(response.data.ltr));
                } else {
                    setEffectiveLtr(false);
                }
                if (response.data.ltr_model !== undefined) {
                    setLtrModel(response.data.ltr_model);
                } else {
                    setLtrModel(null);
                }
                if (response.data.ltr_latency_us !== undefined) {
                    setLtrLatencyUs(response.data.ltr_latency_us);
                } else {
                    setLtrLatencyUs(null);
                }
                if (response.data.price_table_id !== undefined) {
                    setEffectivePriceTableId(response.data.price_table_id);
                } else {
                    setEffectivePriceTableId('');
                }
                if (response.data.sort_by !== undefined) {
                    setEffectiveSortBy(response.data.sort_by);
                } else {
                    setEffectiveSortBy('');
                }
                if (response.data.personalized !== undefined) {
                    setPersonalized(Boolean(response.data.personalized));
                } else {
                    setPersonalized(false);
                }
                if (typeof response.data.sponsored_count === 'number') {
                    setSponsoredCount(response.data.sponsored_count);
                } else {
                    setSponsoredCount(0);
                }
                if (response.data.vector_index_used !== undefined) {
                    setVectorIndexUsed(Boolean(response.data.vector_index_used));
                } else {
                    setVectorIndexUsed(null);
                }
            }

            // Simple processing - just map products without modifying anything
            const processedResults = response.data.results || [];
            const resTotalFound = typeof response.data?.total_found === 'number'
                ? response.data.total_found
                : processedResults.length;
            setTotalFound(resTotalFound);
            setServerTotalPages(
                typeof response.data?.total_pages === 'number' && response.data.total_pages > 0
                    ? response.data.total_pages
                    : Math.max(1, Math.ceil(resTotalFound / pageSizeToUse))
            );
            setServerHasNext(
                typeof response.data?.has_next === 'boolean'
                    ? response.data.has_next
                    : (pageToUse * pageSizeToUse < resTotalFound)
            );
            setServerOffset(
                typeof response.data?.offset === 'number'
                    ? response.data.offset
                    : offsetToUse
            );

            // Debug: Print the first product's image URL to console
            if (processedResults.length > 0 && processedResults[0].images && processedResults[0].images.length > 0) {
                console.log("First product image URL:", processedResults[0].images[0].uri);
            }

            console.log("Products to display:", processedResults.length);

            // Store only necessary product data (without images) in localStorage for details page
            const productsForStorage = processedResults.map(product => {
                // Create a copy of the product without the images array to reduce storage size
                const { images, ...productWithoutImages } = product;
                return {
                    ...productWithoutImages,
                    // Store just a reference to the first image URL if available
                    primaryImageUrl: images && images.length > 0 ? images[0].uri : null
                };
            });
            localStorage.setItem('searchResults', JSON.stringify(productsForStorage));

            // Set products and active page
            setProducts(processedResults);
            setCurrentPage(pageToUse);

            // Show AI filter suggestions if we have products and a search query
            setShowAIFilterSuggestions(processedResults.length > 0 && activeSearchQuery.trim() !== '');
        } catch (error) {
            setAutoCorrected(false);
            setQueryRelaxed(false);
            setDidYouMean('');
            setEffectivePriceTableId('');
            setEffectiveSortBy('');
            setPersonalized(false);
            setSponsoredCount(0);
            setVectorIndexUsed(null);
            setTotalFound(0);
            setServerTotalPages(0);
            setServerHasNext(false);
            setServerOffset(0);
            const serverErrMsg = error.response?.data?.error || error.response?.data?.message;
            const displayError = serverErrMsg || error.message || 'An error occurred while searching';
            setError(displayError);
            setProducts([]);
            console.error("Search error:", error);
        } finally {
            setLoading(false);
        }
    }, [activeSearchQuery, alpha, channel, onlyInStock, selectedCategories, selectedBrands, selectedColors, selectedSizes, selectedPriceRanges, selectedAvailability, tenantId, locale, cep, maxSla24h, ltr, priceTableId, sortBy, productsPerPage]);

    // Handler for filter changes from the Filters component - now works with dynamic filter IDs
    const handleFilterChange = useCallback((filterId, value, isChecked) => {
        setSelectedFilters(prevFilters => {
            const currentSelection = prevFilters[filterId] || [];
            let newSelection;
            if (isChecked) {
                if (currentSelection.some(item => String(item).toLowerCase() === String(value).toLowerCase())) {
                    newSelection = currentSelection;
                } else {
                    newSelection = [...currentSelection, value];
                }
            } else {
                newSelection = currentSelection.filter(
                    item => String(item).toLowerCase() !== String(value).toLowerCase()
                );
            }
            return {
                ...prevFilters,
                [filterId]: newSelection,
            };
        });
        if (!isChecked && filterId === 'colors' && String(color).toLowerCase() === String(value).toLowerCase()) {
            setColor('');
        }
        if (!isChecked && filterId === 'sizes' && String(size).toLowerCase() === String(value).toLowerCase()) {
            setSize('');
        }
        setCurrentPage(1); // Reset to first page when filters change
    }, [color, size]);

    const handleClearAllFilters = useCallback(() => {
        setSelectedFilters({});
        setColor('');
        setSize('');
        setOnlyInStock(false);
        setCurrentPage(1);
    }, []);

    const activeFilterPills = useMemo(() => {
        const pills = [];
        const labelMap = {
            categories: 'Department',
            brands: 'Brand',
            prices: 'Price',
            colors: 'Color',
            sizes: 'Size',
            availability: 'Availability',
        };
        Object.entries(selectedFilters || {}).forEach(([filterId, values]) => {
            if (!Array.isArray(values) || values.length === 0) return;
            const prefix = labelMap[filterId] || (filterId.startsWith('attr_') ? formatFilterName(filterId.substring(5)) : formatFilterName(filterId));
            values.forEach((val) => {
                if (!val) return;
                let displayVal = val;
                if (filterId === 'prices' && typeof val === 'string') {
                    if (val.endsWith('+')) {
                        displayVal = `$${val.slice(0, -1)}+`;
                    } else if (val.includes('-')) {
                        const parts = val.split('-');
                        displayVal = `$${parts[0]} - $${parts[1]}`;
                    }
                }
                pills.push({
                    key: `${filterId}:${val}`,
                    label: `${prefix}: ${displayVal}`,
                    onRemove: () => handleFilterChange(filterId, val, false),
                });
            });
        });
        if (color && !(selectedFilters.colors || []).some(c => String(c).toLowerCase() === String(color).toLowerCase())) {
            pills.push({
                key: `color:${color}`,
                label: `Color: ${color}`,
                onRemove: () => {
                    setColor('');
                    setCurrentPage(1);
                },
            });
        }
        if (size && !(selectedFilters.sizes || []).some(s => String(s).toLowerCase() === String(size).toLowerCase())) {
            pills.push({
                key: `size:${size}`,
                label: `Size: ${size}`,
                onRemove: () => {
                    setSize('');
                    setCurrentPage(1);
                },
            });
        }
        if (onlyInStock) {
            pills.push({
                key: 'only_in_stock',
                label: 'In Stock Only',
                onRemove: () => {
                    setOnlyInStock(false);
                    setCurrentPage(1);
                },
            });
        }
        return pills;
    }, [selectedFilters, color, size, onlyInStock, handleFilterChange]);

    // Dynamic filter logic with optimized memory usage
    const filteredProducts = useMemo(() => {
        if (!products.length || !Object.keys(selectedFilters).length) {
            return products;
        }
        // When serverFacets is active, the backend has already applied disjunctive
        // filters across categories, brands, colors, sizes, and price_ranges.
        if (serverFacets) {
            return products;
        }

        // First identify which filter types are actually being used to avoid unnecessary processing
        const activeFilters = Object.entries(selectedFilters).filter(
            ([_, values]) => values && values.length > 0
        );

        // If no active filters, return all products
        if (activeFilters.length === 0) {
            return products;
        }

        // Create filter functions for each active filter type
        // This avoids recreating them for each product
        const filterFunctions = activeFilters.map(([filterId, selectedValues]) => {
            const lowerSelected = selectedValues.map(v => String(v).toLowerCase());
            switch (filterId) {
                case 'categories':
                    return product =>
                        product.categories &&
                        product.categories.some(cat => lowerSelected.includes(String(cat).toLowerCase()));

                case 'brands':
                    return product => {
                        // Check direct brands first
                        if (product.brands && product.brands.some(brand => lowerSelected.includes(String(brand).toLowerCase()))) {
                            return true;
                        }
                        // Check attribute brands as fallback
                        if (product.attributes) {
                            const brandAttr = product.attributes.find(attr => attr.key === 'brand');
                            return brandAttr && brandAttr.value?.text &&
                                brandAttr.value.text.some(b => lowerSelected.includes(String(b).toLowerCase()));
                        }
                        return false;
                    };

                case 'prices':
                    // Pre-parse the price ranges to avoid repeated parsing
                    const priceRanges = selectedValues.map(range => {
                        if (typeof range === 'string' && range.endsWith('+')) {
                            const min = parseFloat(range.slice(0, -1));
                            return { min, max: Infinity };
                        }
                        const [min, max] = String(range).split('-').map(parseFloat);
                        return { min: isNaN(min) ? 0 : min, max: isNaN(max) ? Infinity : max };
                    });

                    return product => {
                        const price = product.priceInfo?.price ? parseFloat(product.priceInfo.price) : null;
                        if (price === null) return false;
                        return priceRanges.some(range => price >= range.min && price <= range.max);
                    };

                case 'colors':
                    return product => {
                        if (Array.isArray(product.available_colors) &&
                            product.available_colors.some(c => lowerSelected.includes(String(c).toLowerCase()))) {
                            return true;
                        }
                        if (Array.isArray(product.variants) &&
                            product.variants.some(v => v.available && lowerSelected.includes(String(v.color).toLowerCase()))) {
                            return true;
                        }
                        const colorInfo = product.colorInfo;
                        if (!colorInfo) return false;

                        // Check direct colors
                        if (colorInfo.colors && colorInfo.colors.some(c => lowerSelected.includes(String(c).toLowerCase()))) {
                            return true;
                        }
                        // Check color families
                        return colorInfo.colorFamilies &&
                            colorInfo.colorFamilies.some(c => lowerSelected.includes(String(c).toLowerCase()));
                    };

                case 'sizes':
                    return product => {
                        if (Array.isArray(product.available_sizes) &&
                            product.available_sizes.some(s => lowerSelected.includes(String(s).toLowerCase()))) {
                            return true;
                        }
                        if (Array.isArray(product.variants) &&
                            product.variants.some(v => v.available && lowerSelected.includes(String(v.size).toLowerCase()))) {
                            return true;
                        }
                        return product.sizes &&
                            product.sizes.some(sz => lowerSelected.includes(String(sz).toLowerCase()));
                    };

                case 'availability':
                    return product => selectedValues.includes(product.availability);

                default:
                    // Handle dynamic attribute filters (attr_*)
                    if (filterId.startsWith('attr_')) {
                        const attrKey = filterId.substring(5); // Remove 'attr_' prefix
                        return product => {
                            if (!product.attributes) return false;
                            if (Array.isArray(product.attributes)) {
                                const attr = product.attributes.find(a => a.key === attrKey);
                                return attr && attr.value?.text &&
                                    attr.value.text.some(val => lowerSelected.includes(String(val).toLowerCase()));
                            }
                            return false;
                        };
                    }

                    // Default behavior for unknown filter types
                    return () => true;
            }
        });

        // Apply all filter functions to each product
        return products.filter(product =>
            filterFunctions.every(filterFn => filterFn(product))
        );
    }, [products, selectedFilters, serverFacets]);

    // Handle search submission from SearchInput component - now resets all filters
    const handleSearch = useCallback((searchText, options = {}) => {
        const imageChanged = options.image !== undefined && options.image !== queryImageRef.current;
        if (options.image !== undefined) {
            queryImageRef.current = options.image;
        }
        // Reset all filters when a new search is performed
        setSelectedFilters({});
        setServerFacets(null);
        setFacetsLatencyMs(null);
        setExpandedTerms([]);
        setAppliedRules([]);
        if (options.alpha !== undefined) {
            setAlpha(options.alpha);
        }
        if (options.channel !== undefined) {
            setChannel(options.channel);
        }
        if (options.onlyInStock !== undefined) {
            setOnlyInStock(options.onlyInStock);
        }
        if (options.tenantId !== undefined) {
            setTenantId(options.tenantId);
        }
        if (options.locale !== undefined) {
            setLocale(options.locale);
        }
        if (options.cep !== undefined) {
            setCep(options.cep);
        }
        if (options.maxSla24h !== undefined) {
            setMaxSla24h(options.maxSla24h);
        }
        if (options.ltr !== undefined) {
            setLtr(options.ltr);
        }
        if (options.parseIntent !== undefined) {
            setParseIntent(options.parseIntent);
            parseIntentRef.current = options.parseIntent;
        }
        if (options.priceTableId !== undefined) {
            setPriceTableId(options.priceTableId);
        }
        if (options.sortBy !== undefined) {
            setSortBy(options.sortBy);
        }
        if (options.pageSize !== undefined) {
            setProductsPerPage(Number(options.pageSize) || 24);
        }
        if (options.color !== undefined) {
            setColor(options.color);
        }
        if (options.size !== undefined) {
            setSize(options.size);
        }
        setCurrentPage(1);
        setParsedQuery(null);
        setActiveExtractedFilters(null);
        activeExtractedFiltersRef.current = null;
        setCleanQueryBase('');
        cleanQueryBaseRef.current = '';
        setAutoCorrected(false);
        setQueryRelaxed(false);
        setDidYouMean('');
        setResponseQuery('');
        setSponsoredCount(0);
        const nextQuery = typeof searchText === 'string' ? searchText : searchText?.query || '';
        if (nextQuery === activeSearchQuery && (nextQuery.trim() || imageChanged)) {
            fetchProducts({
                query: nextQuery,
                page: 1,
                pageSize: options.pageSize !== undefined ? options.pageSize : productsPerPage,
                parseIntent: options.parseIntent !== undefined ? options.parseIntent : parseIntentRef.current,
                priceTableId: options.priceTableId !== undefined ? options.priceTableId : priceTableId,
                sortBy: options.sortBy !== undefined ? options.sortBy : sortBy,
                color: options.color !== undefined ? options.color : color,
                size: options.size !== undefined ? options.size : size,
            });
        }
        // Update active search query which triggers API call
        setActiveSearchQuery(nextQuery);
        // Reset AI filter suggestions when a new search is performed
        setShowAIFilterSuggestions(true);
    }, [setTenantId, setLocale, fetchProducts, activeSearchQuery, priceTableId, sortBy, productsPerPage, color, size]);

    const handleRemoveExtractedFilter = useCallback((filterKey) => {
        const currentFilters = activeExtractedFiltersRef.current || {};
        const nextFilters = { ...currentFilters };
        delete nextFilters[filterKey];
        if (filterKey === 'price') {
            delete nextFilters.min_price;
            delete nextFilters.max_price;
        }
        setActiveExtractedFilters(nextFilters);
        activeExtractedFiltersRef.current = nextFilters;
        setParseIntent(false);
        parseIntentRef.current = false;
        setCurrentPage(1);

        const q = cleanQueryBaseRef.current || activeSearchQuery;
        fetchProducts({
            query: q,
            page: 1,
            parseIntent: false,
            extractedFilters: nextFilters,
        });
    }, [fetchProducts, activeSearchQuery]);

    useEffect(() => {
        if (activeSearchQuery.trim()) {
            fetchProducts({ page: 1 });
        }

        // Cleanup function to prevent memory leaks
        return () => {
            // Clear any references to large data structures
            setProducts([]);
            setFilterConfigs([]);
            setExpandedTerms([]);
            setAppliedRules([]);
        };
    }, [fetchProducts, activeSearchQuery]);

    const currentProducts = useMemo(() => {
        if (filteredProducts.length > productsPerPage) {
            const indexOfLastProduct = currentPage * productsPerPage;
            const indexOfFirstProduct = indexOfLastProduct - productsPerPage;
            return filteredProducts.slice(indexOfFirstProduct, indexOfLastProduct);
        }
        return filteredProducts;
    }, [filteredProducts, currentPage, productsPerPage]);

    const organicDisplayedCount = useMemo(() => {
        return currentProducts.filter(p => !p || !p.sponsored).length;
    }, [currentProducts]);

    const effectiveTotalFound = totalFound > 0 ? totalFound : filteredProducts.length;
    const effectiveOffset = serverOffset > 0 ? serverOffset : (currentPage - 1) * productsPerPage;
    const displayedCount = organicDisplayedCount || currentProducts.length;
    const showingStart = effectiveTotalFound === 0 || displayedCount === 0 ? 0 : effectiveOffset + 1;
    const showingEnd = effectiveTotalFound === 0 ? 0 : Math.min(effectiveTotalFound, effectiveOffset + displayedCount);
    const totalPages = serverTotalPages > 0
        ? serverTotalPages
        : Math.max(1, Math.ceil(effectiveTotalFound / productsPerPage));

    const handlePageChange = (newPage) => {
        const nextPage = Math.max(1, newPage);
        setCurrentPage(nextPage);
        fetchProducts({ page: nextPage });
    };

    const handlePageSizeChange = (newSize) => {
        const nextSize = Number(newSize) || 24;
        setProductsPerPage(nextSize);
        setCurrentPage(1);
        if (activeSearchQuery.trim() || products.length > 0) {
            fetchProducts({ page: 1, pageSize: nextSize });
        }
    };


    // Prevent search input changes from causing product grid re-renders
    // Only fetch products when activeSearchQuery changes (not during typing)

    // Stable handler for product clicks (fires ad click beacon as fire-and-forget when sponsored)
    const handleProductClick = useCallback((productOrId) => {
        const product = typeof productOrId === 'object' && productOrId !== null ? productOrId : null;
        const productId = product ? product.id : productOrId;
        if (product && product.sponsored && product.ad_campaign_id && product.ad_click_id) {
            recordAdClick(product.ad_campaign_id, product.ad_click_id);
        }
        recordSessionEvent(productId, 'click', tenantId);
        setSessionRefreshKey(prev => prev + 1);
        navigate(`/product/${productId}`);
    }, [navigate, tenantId]);

    const handleSessionUpdate = useCallback((newSessionId) => {
        setSessionId(newSessionId);
        sessionIdRef.current = newSessionId;
        setSessionRefreshKey(prev => prev + 1);
        fetchProducts({ sessionId: newSessionId });
    }, [fetchProducts]);

    // Optimized product grid with exactly 6 products per row
    const ProductGrid = memo(({ products, onProductClick }) => {
        return (
            <Grid 
                container 
                sx={{ 
                    width: '100%', 
                    margin: 0,
                    display: 'flex',
                    flexWrap: 'wrap',
                    marginLeft: '-8px',
                    marginRight: '-8px'
                }}
            >
                {products.map((product) => {
                    // Extract just the minimal image data needed for this specific product
                    // This is done inline instead of processing all products at once
                    const imageUrl = product.images &&
                        Array.isArray(product.images) &&
                        product.images.length > 0 &&
                        product.images[0].uri ?
                        product.images[0].uri :
                        null;
                    const isSponsored = Boolean(product.sponsored);
                    const cardKey = product.ad_click_id ? `${product.id}-${product.ad_click_id}` : product.id;

                    return (
                        <Grid 
                            item 
                            key={cardKey}
                            sx={{ 
                                width: 'calc(16.666% - 16px)',
                                padding: '8px',
                                boxSizing: 'border-box'
                            }}
                        >
                            <Card
                                data-testid={isSponsored ? 'sponsored-card' : 'product-card'}
                                data-sponsored={isSponsored ? 'true' : 'false'}
                                sx={{
                                    minHeight: '300px',
                                    display: 'flex',
                                    flexDirection: 'column',
                                    cursor: 'pointer',
                                    transition: 'box-shadow 0.3s ease',
                                    ...(isSponsored ? {
                                        border: '1px solid #f59e0b',
                                        backgroundColor: '#fffbeb',
                                        boxShadow: '0 2px 8px rgba(245, 158, 11, 0.15)',
                                    } : {}),
                                    '&:hover': {
                                        boxShadow: 6,
                                    }
                                }}
                                onClick={() => onProductClick(product)}
                            >
                                {/* Each product processes its own image data */}
                                <ProductImage
                                    imageUrl={imageUrl}
                                    productName={product.name}
                                />
                                <CardContent sx={{ 
                                    flexGrow: 1,
                                    display: 'flex',
                                    flexDirection: 'column',
                                    justifyContent: 'space-between',
                                    padding: '8px',
                                }}>
                                    <Box>
                                        {isSponsored && (
                                            <Box sx={{ mb: 0.75, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                                                <Chip
                                                    label="Patrocinado"
                                                    size="small"
                                                    data-testid="sponsored-badge"
                                                    sx={{
                                                        height: 20,
                                                        fontSize: '0.7rem',
                                                        fontWeight: 700,
                                                        backgroundColor: '#fef3c7',
                                                        color: '#92400e',
                                                        border: '1px solid #f59e0b',
                                                    }}
                                                />
                                            </Box>
                                        )}
                                        <Typography 
                                            gutterBottom 
                                            variant="subtitle2" 
                                            component="div"
                                            sx={{
                                                height: '38px', // Reduced height for title
                                                overflow: 'hidden',
                                                textOverflow: 'ellipsis',
                                                display: '-webkit-box',
                                                WebkitLineClamp: 2,
                                                WebkitBoxOrient: 'vertical'
                                            }}
                                        >
                                            {product.name}
                                        </Typography>
                                        <Typography 
                                            variant="body2" 
                                            color="text.secondary" 
                                            gutterBottom
                                            sx={{
                                                height: '40px', // Fixed height for description
                                                overflow: 'hidden',
                                                textOverflow: 'ellipsis',
                                                display: '-webkit-box',
                                                WebkitLineClamp: 2,
                                                WebkitBoxOrient: 'vertical'
                                            }}
                                        >
                                            {product.title}
                                        </Typography>
                                        
                                        {/* Categories with a fixed height container */}
                                        <Box sx={{ 
                                            height: '36px', 
                                            overflowY: 'hidden',
                                            display: 'flex',
                                            flexWrap: 'wrap',
                                            alignItems: 'flex-start'
                                        }}>
                                            {product.categories && product.categories.slice(0, 2).map(category => (
                                                <Chip
                                                    key={category}
                                                    label={category}
                                                    size="small"
                                                    sx={{ mr: 0.5, mb: 0.5 }}
                                                />
                                            ))}
                                        </Box>
                                    </Box>

                                    {/* Price and availability at the bottom */}
                                    <Box sx={{ mt: 'auto' }}>
                                        <Typography variant="h6" color="primary">
                                            ${product.priceInfo?.price}
                                        </Typography>
                                        {product.priceInfo?.priceTableId ? (
                                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, flexWrap: 'wrap', mb: 0.5 }}>
                                                <Typography
                                                    variant="body2"
                                                    color="text.secondary"
                                                    sx={{ textDecoration: 'line-through' }}
                                                >
                                                    ${product.priceInfo?.basePrice !== undefined && product.priceInfo?.basePrice !== null
                                                        ? Number(product.priceInfo.basePrice).toFixed(2)
                                                        : (product.priceInfo?.originalPrice || '')}
                                                </Typography>
                                                <Chip
                                                    label={`Contrato: ${product.priceInfo.priceTableId}`}
                                                    size="small"
                                                    color="secondary"
                                                    variant="outlined"
                                                    sx={{ height: 20, fontSize: '0.7rem' }}
                                                />
                                            </Box>
                                        ) : (product.priceInfo?.originalPrice &&
                                            product.priceInfo.originalPrice !== "0" &&
                                            parseFloat(product.priceInfo.originalPrice) > parseFloat(product.priceInfo.price) && (
                                                <Typography
                                                    variant="body2"
                                                    color="text.secondary"
                                                    sx={{ textDecoration: 'line-through' }}
                                                >
                                                    ${product.priceInfo.originalPrice}
                                                </Typography>
                                            ))}
                                        {(() => {
                                            const availColors = Array.isArray(product.available_colors) ? product.available_colors : [];
                                            const availSizes = Array.isArray(product.available_sizes) ? product.available_sizes : [];
                                            const variantCount = typeof product.variant_count === 'number'
                                                ? product.variant_count
                                                : (Array.isArray(product.variants) ? product.variants.length : 0);
                                            const displayedCount = availColors.length + availSizes.length;
                                            const extraVariants = variantCount > displayedCount ? variantCount - displayedCount : 0;
                                            if (availColors.length === 0 && availSizes.length === 0 && extraVariants === 0) {
                                                return null;
                                            }
                                            return (
                                                <Box sx={{ display: 'flex', gap: 0.5, alignItems: 'center', flexWrap: 'wrap', mt: 0.75 }}>
                                                    {availColors.map((c) => {
                                                        const swatchHex = COLOR_SWATCH_MAP[String(c).toLowerCase()] || '#64748b';
                                                        return (
                                                            <Chip
                                                                key={`color-${c}`}
                                                                label={c}
                                                                size="small"
                                                                variant="outlined"
                                                                icon={
                                                                    <Box
                                                                        component="span"
                                                                        sx={{
                                                                            width: 10,
                                                                            height: 10,
                                                                            borderRadius: '50%',
                                                                            backgroundColor: swatchHex,
                                                                            border: '1px solid #94a3b8',
                                                                            display: 'inline-block',
                                                                            ml: '6px !important',
                                                                        }}
                                                                    />
                                                                }
                                                                sx={{ height: 22, fontSize: '0.7rem' }}
                                                            />
                                                        );
                                                    })}
                                                    {availSizes.map((sz) => (
                                                        <Chip
                                                            key={`size-${sz}`}
                                                            label={sz}
                                                            size="small"
                                                            variant="outlined"
                                                            sx={{ height: 22, fontSize: '0.7rem', fontWeight: 600 }}
                                                        />
                                                    ))}
                                                    {extraVariants > 0 && (
                                                        <Chip
                                                            label={`+${extraVariants} variantes`}
                                                            size="small"
                                                            color="primary"
                                                            variant="outlined"
                                                            sx={{ height: 22, fontSize: '0.7rem', fontWeight: 600 }}
                                                        />
                                                    )}
                                                </Box>
                                            );
                                        })()}
                                        {(() => {
                                            const isStockIn = product.availability === 'IN_STOCK';
                                            const stockBadge = isStockIn
                                                ? (product.availableQuantity !== undefined && product.availableQuantity !== null
                                                    ? `IN_STOCK (${product.availableQuantity} unid.)`
                                                    : 'IN_STOCK')
                                                : 'OUT_OF_STOCK';
                                            const prodLocale = product.locale || product.Locale;
                                            const dp = product.deliveryPromise;
                                            return (
                                                <Box sx={{ display: 'flex', gap: 0.5, alignItems: 'center', flexWrap: 'wrap', mt: 1 }}>
                                                    <Chip
                                                        label={stockBadge}
                                                        color={isStockIn ? 'success' : 'error'}
                                                        size="small"
                                                    />
                                                    {prodLocale && (
                                                        <Chip
                                                            label={prodLocale}
                                                            size="small"
                                                            variant="outlined"
                                                            color="info"
                                                            sx={{ fontWeight: 600, height: 24, fontSize: '0.75rem' }}
                                                        />
                                                    )}
                                                    {dp && dp.label && (
                                                        <Chip
                                                            label={dp.label}
                                                            size="small"
                                                            color="success"
                                                            sx={{ fontWeight: 600, height: 24, fontSize: '0.75rem' }}
                                                        />
                                                    )}
                                                    {dp && dp.free_shipping && (
                                                        <Chip
                                                            label="Frete grátis"
                                                            size="small"
                                                            variant="outlined"
                                                            color="success"
                                                            sx={{ fontWeight: 600, height: 24, fontSize: '0.75rem' }}
                                                        />
                                                    )}
                                                    {(() => {
                                                        const s = product.score || {};
                                                        if (s.stage1_rank !== undefined && s.stage2_rank !== undefined) {
                                                            const diff = s.stage1_rank - s.stage2_rank;
                                                            let rankLabel = `${s.stage1_rank} → ${s.stage2_rank}`;
                                                            if (diff > 0) rankLabel += ` (↑${diff})`;
                                                            else if (diff < 0) rankLabel += ` (↓${Math.abs(diff)})`;
                                                            const color = diff > 0 ? 'success' : diff < 0 ? 'warning' : 'default';
                                                            return (
                                                                <Chip
                                                                    label={rankLabel}
                                                                    size="small"
                                                                    color={color}
                                                                    variant="outlined"
                                                                    sx={{ fontWeight: 600, height: 24, fontSize: '0.75rem' }}
                                                                />
                                                            );
                                                        }
                                                        return null;
                                                    })()}
                                                    {(() => {
                                                        const s = product.score || {};
                                                        if (personalized && s.session_boost !== undefined) {
                                                            const b = Number(s.session_boost);
                                                            return (
                                                                <Chip
                                                                    label={`boost: +${b.toFixed(4)}`}
                                                                    size="small"
                                                                    color={b > 0 ? "secondary" : "default"}
                                                                    variant="outlined"
                                                                    sx={{ fontWeight: 600, height: 24, fontSize: '0.75rem' }}
                                                                />
                                                            );
                                                        }
                                                        return null;
                                                    })()}
                                                </Box>
                                            );
                                        })()}
                                    </Box>
                                </CardContent>
                            </Card>
                        </Grid>
                    );
                })}
            </Grid>
        );
    });

    return (
        <Container maxWidth="lg" sx={{ mt: 4 }}>
            <Grid container spacing={3} sx={{ 
                display: 'flex', 
                flexWrap: { xs: 'wrap', sm: 'nowrap' }
            }}>
                {/* Filters Section */}
                <Grid item xs={12} sm={4} md={3} sx={{ 
                    flex: '0 0 auto',
                    maxWidth: { xs: '100%', sm: '33.333%', md: '25%' },
                    display: { xs: 'none', sm: 'block' } 
                }}>
                    <SessionPanel
                        onSessionUpdate={handleSessionUpdate}
                        tenantId={tenantId}
                        refreshKey={sessionRefreshKey}
                    />
                    <Card sx={{
                        p: 2,
                        boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
                        borderRadius: '8px',
                        position: 'sticky',
                        top: '16px',
                    }}>
                        <Typography
                            variant="h6"
                            component="div"
                            sx={{
                                mb: 3,
                                fontWeight: 600,
                                fontSize: '20px',
                                color: '#212121'
                            }}
                        >
                            Filters
                        </Typography>

                        {/* Render the dynamic Filters component */}
                        <Filters
                            filterConfigs={filterConfigs}
                            selectedFilters={selectedFilters}
                            onFilterChange={handleFilterChange}
                            onClearAll={handleClearAllFilters}
                            facetsLatencyMs={facetsLatencyMs}
                        />
                    </Card>
                </Grid>

                {/* Filters Section - Mobile view */}
                <Grid item sx={{ 
                    display: { xs: 'block', sm: 'none' },
                    width: '100%',
                    mb: 2
                }}>
                    <SessionPanel
                        onSessionUpdate={handleSessionUpdate}
                        tenantId={tenantId}
                        refreshKey={sessionRefreshKey}
                    />
                    <Card sx={{
                        p: 2,
                        boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
                        borderRadius: '8px',
                    }}>
                        <Typography
                            variant="h6"
                            component="div"
                            sx={{
                                mb: 3,
                                fontWeight: 600,
                                fontSize: '20px',
                                color: '#212121'
                            }}
                        >
                            Filters
                        </Typography>

                        {/* Render the dynamic Filters component */}
                        <Filters
                            filterConfigs={filterConfigs}
                            selectedFilters={selectedFilters}
                            onFilterChange={handleFilterChange}
                            onClearAll={handleClearAllFilters}
                            facetsLatencyMs={facetsLatencyMs}
                        />
                    </Card>
                </Grid>

                {/* Search and Products Section */}
                <Grid item xs={12} sm={8} md={9} sx={{ 
                    flex: '1 1 auto',
                    maxWidth: { xs: '100%', sm: '66.666%', md: '75%' } 
                }}>
                    {/* Use the isolated SearchInput component */}
                    <SearchInput
                        onSearch={handleSearch}
                        query={activeSearchQuery}
                        alpha={alpha}
                        setAlpha={setAlpha}
                        channel={channel}
                        setChannel={setChannel}
                        onlyInStock={onlyInStock}
                        setOnlyInStock={setOnlyInStock}
                        tenantId={tenantId}
                        setTenantId={setTenantId}
                        locale={locale}
                        setLocale={setLocale}
                        cep={cep}
                        setCep={setCep}
                        maxSla24h={maxSla24h}
                        setMaxSla24h={setMaxSla24h}
                        ltr={ltr}
                        setLtr={setLtr}
                        parseIntent={parseIntent}
                        setParseIntent={(val) => {
                            setParseIntent(val);
                            parseIntentRef.current = val;
                        }}
                        priceTableId={priceTableId}
                        setPriceTableId={setPriceTableId}
                        sortBy={sortBy}
                        setSortBy={(val) => {
                            setCurrentPage(1);
                            setSortBy(val);
                        }}
                        pageSize={productsPerPage}
                        setPageSize={handlePageSizeChange}
                        color={color}
                        setColor={setColor}
                        size={size}
                        setSize={setSize}
                    />

                    {/* Display search result count and search metadata */}
                    {/* Results container with min-height to prevent layout shifts */}
                    <Box sx={{
                        minHeight: '50vh',
                        transition: 'all 0.3s ease',  /* Add transition for smooth changes */
                    }}>
                        {/* AI Filter Suggestions Component - updated to use filterConfigs */}
                        <AIFilterSuggestion
                            filterConfigs={filterConfigs}
                            selectedFilters={selectedFilters}
                            onFilterChange={handleFilterChange}
                            isVisible={showAIFilterSuggestions}
                            searchQuery={activeSearchQuery}
                            allProducts={products} // Pass all products for smart filtering
                        />

                        {/* Active multi-select filter pills + Clear all (Issue #41, ADR-013) */}
                        {activeFilterPills.length > 0 && (
                            <Box
                                data-testid="active-filter-pills"
                                sx={{ mb: 2, display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}
                            >
                                <Typography variant="body2" sx={{ fontWeight: 600, color: 'text.secondary', mr: 0.5 }}>
                                    Active filters:
                                </Typography>
                                {activeFilterPills.map((pill) => (
                                    <Chip
                                        key={pill.key}
                                        size="small"
                                        color="primary"
                                        variant="outlined"
                                        label={pill.label}
                                        onDelete={pill.onRemove}
                                    />
                                ))}
                                <Button
                                    size="small"
                                    variant="text"
                                    color="secondary"
                                    onClick={handleClearAllFilters}
                                    sx={{ textTransform: 'none', fontWeight: 600, ml: 0.5 }}
                                >
                                    Clear all
                                </Button>
                            </Box>
                        )}

                        {/* Zero-results recovery banner (Issue #21) */}
                        {autoCorrected && didYouMean ? (
                            <Alert severity="info" sx={{ mb: 2, alignItems: 'center' }}>
                                <span>Mostrando resultados para &quot;{didYouMean}&quot;. </span>
                                <Button
                                    variant="text"
                                    color="primary"
                                    size="small"
                                    onClick={() => {
                                        const orig = responseQuery || activeSearchQuery;
                                        setParseIntent(false);
                                        parseIntentRef.current = false;
                                        setActiveSearchQuery(orig);
                                        fetchProducts({
                                            query: orig,
                                            page: 1,
                                            parseIntent: false,
                                            extractedFilters: null,
                                        });
                                    }}
                                    sx={{
                                        textTransform: 'none',
                                        p: 0,
                                        minWidth: 'auto',
                                        verticalAlign: 'baseline',
                                        fontWeight: 600,
                                        textDecoration: 'underline',
                                        cursor: 'pointer',
                                    }}
                                >
                                    Buscar por &quot;{responseQuery || activeSearchQuery}&quot;
                                </Button>
                            </Alert>
                        ) : !autoCorrected && didYouMean ? (
                            <Alert severity="info" sx={{ mb: 2, alignItems: 'center' }}>
                                <span>Você quis dizer: </span>
                                <Button
                                    variant="text"
                                    color="primary"
                                    size="small"
                                    onClick={() => {
                                        setActiveSearchQuery(didYouMean);
                                        fetchProducts({
                                            query: didYouMean,
                                            page: 1,
                                            parseIntent: parseIntentRef.current,
                                            extractedFilters: null,
                                        });
                                    }}
                                    sx={{
                                        textTransform: 'none',
                                        p: 0,
                                        minWidth: 'auto',
                                        verticalAlign: 'baseline',
                                        fontWeight: 600,
                                        textDecoration: 'underline',
                                        cursor: 'pointer',
                                    }}
                                >
                                    {didYouMean}?
                                </Button>
                            </Alert>
                        ) : queryRelaxed ? (
                            <Alert severity="info" sx={{ mb: 2 }}>
                                Nenhum resultado com todos os termos — mostrando resultados com qualquer um deles
                            </Alert>
                        ) : null}

                        {/* Smart query intent chips (Issue #19) */}
                        {activeExtractedFilters && Object.keys(activeExtractedFilters).length > 0 && (
                            <Box sx={{ mb: 2, display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                                <Typography variant="body2" sx={{ fontWeight: 600, color: 'text.secondary', mr: 0.5 }}>
                                    Smart query:
                                </Typography>
                                {cleanQueryBase && (
                                    <Chip
                                        size="small"
                                        color="primary"
                                        variant="outlined"
                                        label={`Query: "${cleanQueryBase}"`}
                                    />
                                )}
                                {activeExtractedFilters.min_price !== undefined && activeExtractedFilters.min_price !== null && Number(activeExtractedFilters.min_price) > 0 && (
                                    <Chip
                                        size="small"
                                        color="primary"
                                        label={`Preço ≥ ${activeExtractedFilters.min_price}`}
                                        onDelete={() => handleRemoveExtractedFilter('min_price')}
                                    />
                                )}
                                {activeExtractedFilters.max_price !== undefined && activeExtractedFilters.max_price !== null && Number(activeExtractedFilters.max_price) > 0 && (
                                    <Chip
                                        size="small"
                                        color="primary"
                                        label={`Preço ≤ ${activeExtractedFilters.max_price}`}
                                        onDelete={() => handleRemoveExtractedFilter('max_price')}
                                    />
                                )}
                                {activeExtractedFilters.brand && (
                                    <Chip
                                        size="small"
                                        color="primary"
                                        label={`Marca: ${activeExtractedFilters.brand}`}
                                        onDelete={() => handleRemoveExtractedFilter('brand')}
                                    />
                                )}
                                {activeExtractedFilters.category && (
                                    <Chip
                                        size="small"
                                        color="primary"
                                        label={`Categoria: ${activeExtractedFilters.category}`}
                                        onDelete={() => handleRemoveExtractedFilter('category')}
                                    />
                                )}
                                {activeExtractedFilters.color && (
                                    <Chip
                                        size="small"
                                        variant="outlined"
                                        label={`Cor: ${activeExtractedFilters.color}`}
                                    />
                                )}
                                {parsedQuery?.intent_source && parsedQuery.intent_source !== 'none' && (
                                    <Chip
                                        size="small"
                                        variant="outlined"
                                        sx={{ fontStyle: 'italic', opacity: 0.8 }}
                                        label={`(${parsedQuery.intent_source})`}
                                    />
                                )}
                            </Box>
                        )}

                        <Box sx={{ mb: 2, display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 1 }}>
                            <Typography variant="body2" color="text.secondary" data-testid="pagination-summary">
                                {currentProducts.length
                                    ? `Showing ${showingStart}-${showingEnd} of ${effectiveTotalFound} results`
                                    : 'No products found. Try a search!'}
                                {effectiveAlpha !== null && ` • effective_alpha: ${effectiveAlpha}`}
                                {retrievalMode && ` • retrieval_mode: ${retrievalMode}`}
                                {vectorIndexUsed !== null && ` • vector_index_used: ${vectorIndexUsed}`}
                                {effectiveTenantId && ` • tenant: ${effectiveTenantId}`}
                                {effectiveLocale && ` • locale: ${effectiveLocale}`}
                                {facetsLatencyMs !== null && ` • facets: ${Number(facetsLatencyMs).toFixed(1)}ms`}
                                {appliedRules && appliedRules.length > 0 && ` • applied_rules: ${appliedRules.join(', ')}`}
                                {effectiveLtr && ltrModel && ` • ltr: ${ltrModel} (${ltrLatencyUs || 0}µs)`}
                                {parsedQuery && parsedQuery.intent_source && parsedQuery.intent_source !== 'none' && ` • intent: ${parsedQuery.intent_source}`}
                                {effectivePriceTableId && ` • price_table: ${effectivePriceTableId}`}
                                {effectiveSortBy && ` • sort: ${effectiveSortBy}`}
                                {personalized && ` • Personalizado`}
                                {sponsoredCount > 0 && ` • sponsored: ${sponsoredCount}`}
                            </Typography>
                            <Box sx={{ display: 'flex', gap: 1, alignItems: 'center', flexWrap: 'wrap' }}>
                                {vectorIndexUsed !== null && (
                                    <Chip
                                        size="small"
                                        color={vectorIndexUsed ? 'success' : 'default'}
                                        variant="outlined"
                                        label={vectorIndexUsed ? 'ANN: ScaNN' : 'ANN: exact'}
                                    />
                                )}
                                {sponsoredCount > 0 && (
                                    <Chip
                                        size="small"
                                        label={`Patrocinados: ${sponsoredCount}`}
                                        sx={{
                                            backgroundColor: '#fef3c7',
                                            color: '#92400e',
                                            border: '1px solid #f59e0b',
                                            fontWeight: 600,
                                        }}
                                    />
                                )}
                                {personalized && (
                                    <Chip size="small" color="secondary" label="Personalizado" />
                                )}
                                {effectiveAlpha !== null && (
                                    <Chip size="small" variant="outlined" label={`effective_alpha: ${effectiveAlpha}`} />
                                )}
                                {retrievalMode && (
                                    <Chip size="small" color="primary" variant="outlined" label={`retrieval_mode: ${retrievalMode}`} />
                                )}
                                {effectiveTenantId && (
                                    <Chip size="small" variant="outlined" label={`tenant: ${effectiveTenantId}`} />
                                )}
                                {effectiveLocale && (
                                    <Chip size="small" color="info" variant="outlined" label={`locale: ${effectiveLocale}`} />
                                )}
                                {facetsLatencyMs !== null && (
                                    <Chip size="small" variant="outlined" label={`facets: ${Number(facetsLatencyMs).toFixed(1)}ms`} />
                                )}
                                {appliedRules && appliedRules.length > 0 && (
                                    <Chip size="small" color="secondary" variant="outlined" label={`applied_rules: ${appliedRules.join(', ')}`} />
                                )}
                                {effectiveLtr && ltrModel && (
                                    <Chip size="small" color="secondary" variant="outlined" label={`ltr_model: ${ltrModel}`} />
                                )}
                                {effectiveLtr && ltrLatencyUs !== null && ltrLatencyUs !== undefined && (
                                    <Chip size="small" variant="outlined" label={`ltr_latency: ${ltrLatencyUs}µs`} />
                                )}
                                {parsedQuery && parsedQuery.intent_source && parsedQuery.intent_source !== 'none' && (
                                    <Chip size="small" color="info" variant="outlined" label={`intent: ${parsedQuery.intent_source}`} />
                                )}
                                {effectivePriceTableId && (
                                    <Chip size="small" color="primary" variant="outlined" label={`price_table: ${effectivePriceTableId}`} />
                                )}
                                {effectiveSortBy && (
                                    <Chip size="small" variant="outlined" label={`sort: ${effectiveSortBy}`} />
                                )}
                            </Box>
                        </Box>

                        {expandedTerms && expandedTerms.length > 0 && (
                            <Box sx={{ mb: 1.5, display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                                <Typography variant="body2" color="text.secondary" sx={{ fontStyle: 'italic' }}>
                                    Also matching: {expandedTerms.join(', ')}
                                </Typography>
                            </Box>
                        )}

                        {appliedRules && appliedRules.length > 0 && (
                            <Box sx={{ mb: 1.5, display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                                <Typography variant="body2" color="text.secondary">
                                    Applied rules: {appliedRules.join(', ')}
                                </Typography>
                            </Box>
                        )}

                        {loading && <Typography>Loading products...</Typography>}
                        {error && (
                            <Alert severity="error" sx={{ mb: 2 }}>
                                {typeof error === 'string' ? error : (error.message || 'An error occurred')}
                            </Alert>
                        )}

                        {/* Use memoized product grid component with stable click handler */}
                        <ProductGrid
                            products={currentProducts}
                            onProductClick={handleProductClick}
                        />
                    </Box>

                    {/* Pagination Controls */}
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mt: 2, flexWrap: 'wrap', gap: 2 }}>
                        <Typography variant="body2" color="text.secondary">
                            {`Showing ${showingStart}-${showingEnd} of ${effectiveTotalFound} results`}
                        </Typography>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                            <Button
                                disabled={currentPage <= 1}
                                onClick={() => handlePageChange(currentPage - 1)}
                            >
                                Previous
                            </Button>

                            {/* Advanced pagination with ellipsis */}
                            {(() => {
                                const pageButtons = [];
                                const showEllipsisStart = currentPage > 4;
                                const showEllipsisEnd = currentPage < totalPages - 3;

                                for (let i = 1; i <= totalPages; i++) {
                                    if (i === 1 || i === totalPages) {
                                        pageButtons.push(
                                            <Button
                                                key={i}
                                                onClick={() => handlePageChange(i)}
                                                variant={currentPage === i ? 'contained' : 'text'}
                                                color={currentPage === i ? 'primary' : 'inherit'}
                                                sx={{ minWidth: '36px' }}
                                            >
                                                {i}
                                            </Button>
                                        );
                                        continue;
                                    }

                                    if (i >= currentPage - 1 && i <= currentPage + 1) {
                                        pageButtons.push(
                                            <Button
                                                key={i}
                                                onClick={() => handlePageChange(i)}
                                                variant={currentPage === i ? 'contained' : 'text'}
                                                color={currentPage === i ? 'primary' : 'inherit'}
                                                sx={{ minWidth: '36px' }}
                                            >
                                                {i}
                                            </Button>
                                        );
                                        continue;
                                    }

                                    if (i < 5 && currentPage < 6) {
                                        pageButtons.push(
                                            <Button
                                                key={i}
                                                onClick={() => handlePageChange(i)}
                                                variant={currentPage === i ? 'contained' : 'text'}
                                                color={currentPage === i ? 'primary' : 'inherit'}
                                                sx={{ minWidth: '36px' }}
                                            >
                                                {i}
                                            </Button>
                                        );
                                        continue;
                                    }

                                    if (i > totalPages - 4 && currentPage > totalPages - 5) {
                                        pageButtons.push(
                                            <Button
                                                key={i}
                                                onClick={() => handlePageChange(i)}
                                                variant={currentPage === i ? 'contained' : 'text'}
                                                color={currentPage === i ? 'primary' : 'inherit'}
                                                sx={{ minWidth: '36px' }}
                                            >
                                                {i}
                                            </Button>
                                        );
                                        continue;
                                    }

                                    if (i === 2 && showEllipsisStart) {
                                        pageButtons.push(
                                            <Box key="ellipsis-start" sx={{ mx: 1 }}>
                                                ...
                                            </Box>
                                        );
                                    }

                                    if (i === totalPages - 1 && showEllipsisEnd) {
                                        pageButtons.push(
                                            <Box key="ellipsis-end" sx={{ mx: 1 }}>
                                                ...
                                            </Box>
                                        );
                                    }
                                }

                                return pageButtons;
                            })()}

                            <Button
                                disabled={(!serverHasNext && currentPage >= totalPages) || totalPages === 0}
                                onClick={() => handlePageChange(currentPage + 1)}
                            >
                                Next
                            </Button>
                        </Box>
                        <FormControl size="small" sx={{ minWidth: 110 }}>
                            <InputLabel id="bottom-page-size-label">Per page</InputLabel>
                            <Select
                                labelId="bottom-page-size-label"
                                value={productsPerPage}
                                label="Per page"
                                onChange={(e) => handlePageSizeChange(e.target.value)}
                            >
                                <MenuItem value={12}>12</MenuItem>
                                <MenuItem value={24}>24</MenuItem>
                                <MenuItem value={48}>48</MenuItem>
                            </Select>
                        </FormControl>
                    </Box>
                </Grid>
            </Grid>
        </Container>
    );
}

// Create a theme with Pic2Catalog colors
const theme = createTheme({
    palette: {
        primary: {
            main: '#4285F4', // Blue color from Pic2Catalog
            contrastText: '#fff',
        },
        secondary: {
            main: '#f5f5f5', // Light gray for backgrounds
        },
        background: {
            default: '#f5f5f5',
            paper: '#ffffff',
        },
        text: {
            primary: '#333333',
            secondary: '#666666',
        },
    },
    components: {
        MuiButton: {
            styleOverrides: {
                contained: {
                    backgroundColor: '#4285F4',
                    '&:hover': {
                        backgroundColor: '#3367d6',
                    },
                },
            },
        },
        MuiCard: {
            styleOverrides: {
                root: {
                    boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
                    borderRadius: '8px',
                },
            },
        },
    },
});

function App() {
    const [tenantId, setTenantId] = useState('default');
    const [locale, setLocale] = useState('');

    return (
        <ThemeProvider theme={theme}>
            <Router>
                <Box sx={{ flexGrow: 1, bgcolor: 'background.default', minHeight: '100vh' }}>
                    <AppBar position="static" sx={{ backgroundColor: '#4285F4' }}>
                        <Toolbar sx={{ display: 'flex', flexWrap: 'wrap', gap: 2 }}>
                            <Typography variant="h6" component="div" sx={{ flexGrow: 1 }}>
                                PSearch
                            </Typography>
                            <TextField
                                id="header-tenant-input"
                                label="Tenant"
                                size="small"
                                value={tenantId}
                                onChange={(e) => setTenantId(e.target.value)}
                                inputProps={{ 'aria-label': 'Tenant' }}
                                sx={{
                                    width: 120,
                                    '& .MuiInputBase-root': { color: 'white', bgcolor: 'rgba(255,255,255,0.15)' },
                                    '& .MuiInputLabel-root': { color: 'rgba(255,255,255,0.85)' },
                                    '& .MuiOutlinedInput-notchedOutline': { borderColor: 'rgba(255,255,255,0.4)' },
                                    '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: 'white' },
                                }}
                            />
                            <FormControl size="small" sx={{
                                minWidth: 120,
                                '& .MuiInputBase-root': { color: 'white', bgcolor: 'rgba(255,255,255,0.15)' },
                                '& .MuiInputLabel-root': { color: 'rgba(255,255,255,0.85)' },
                                '& .MuiOutlinedInput-notchedOutline': { borderColor: 'rgba(255,255,255,0.4)' },
                                '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: 'white' },
                                '& .MuiSvgIcon-root': { color: 'white' },
                            }}>
                                <InputLabel id="header-locale-select-label" sx={{ color: 'rgba(255,255,255,0.85)' }}>Locale</InputLabel>
                                <Select
                                    labelId="header-locale-select-label"
                                    id="header-locale-select"
                                    value={locale}
                                    label="Locale"
                                    onChange={(e) => setLocale(e.target.value)}
                                    inputProps={{ 'aria-label': 'Locale' }}
                                >
                                    <MenuItem value="">All</MenuItem>
                                    <MenuItem value="pt-BR">pt-BR</MenuItem>
                                    <MenuItem value="en-US">en-US</MenuItem>
                                    <MenuItem value="es-MX">es-MX</MenuItem>
                                </Select>
                            </FormControl>
                            <Button
                                component={Link}
                                to="/"
                                color="inherit"
                                startIcon={<SearchIcon />}
                            >
                                Search
                            </Button>
                            <Button
                                component={Link}
                                to="/source-ingestion"
                                color="inherit"
                                startIcon={<FileUploadIcon />}
                            >
                                Source Ingestion
                            </Button>
                            <Button
                                component={Link}
                                to="/manage"
                                color="inherit"
                                startIcon={<SettingsIcon />}
                            >
                                Manage Rules
                            </Button>
                            <Button
                                component={Link}
                                to="/ads"
                                color="inherit"
                                startIcon={<CampaignIcon />}
                            >
                                Sponsored Ads
                            </Button>
                        </Toolbar>
                    </AppBar>

                    <Routes>
                        <Route path="/" element={<ProductSearch tenantId={tenantId} setTenantId={setTenantId} locale={locale} setLocale={setLocale} />} />
                        <Route path="/source-ingestion" element={<SourceIngestion />} />
                        <Route path="/manage" element={<RuleManager />} />
                        <Route path="/ads" element={<AdCampaignManager tenantId={tenantId} />} />
                        <Route path="/product/:productId" element={<ProductDetails />} />
                    </Routes>
                </Box>
            </Router>
        </ThemeProvider>
    );
}

export default App;
