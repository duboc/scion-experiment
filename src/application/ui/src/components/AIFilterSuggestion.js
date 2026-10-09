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

import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { Box, Typography, Button, Paper, Chip, Stack, CircularProgress } from '@mui/material';
import UndoIcon from '@mui/icons-material/Undo';
import geminiIcon from '../assets/gemini.png';
import { getConversationalSearch } from '../services/genAiService';

// Gemini Icon Component - moved outside to prevent recreation
const GeminiIcon = React.memo(() => (
    <img
        src={geminiIcon}
        alt="Gemini AI"
        style={{
            width: '20px',
            height: '20px',
            display: 'block',
            margin: '0 auto'
        }}
    />
));

/**
 * Optimized AI-powered filter suggestion component
 * Memory-efficient implementation with proper memoization
 */
const AIFilterSuggestion = ({
    filterConfigs = [],
    selectedFilters,
    onFilterChange,
    isVisible,
    searchQuery,
    allProducts
}) => {
    // Track current question index and filter history
    const [currentQuestionIndex, setCurrentQuestionIndex] = useState(0);
    const [filterHistory, setFilterHistory] = useState([]);

    // State for conversational search
    const [aiQuestions, setAiQuestions] = useState([]);
    const [conversationHistory, setConversationHistory] = useState([]);
    const [isLoadingAI, setIsLoadingAI] = useState(false);
    const [aiQuestionMap, setAiQuestionMap] = useState({});

    // State for questions - now properly managed
    const [questions, setQuestions] = useState([]);

    // Reset component state when a new search is performed
    useEffect(() => {
        setCurrentQuestionIndex(0);
        setFilterHistory([]);

        if (searchQuery) {
            fetchAIFilterSuggestions(searchQuery);
        } else {
            setAiQuestions([]);
            setConversationHistory([]);
        }
    }, [searchQuery]);

    // OPTIMIZATION: Memoize filtered products calculation to prevent recalculation
    const filteredProducts = useMemo(() => {
        if (!allProducts || !selectedFilters) return allProducts || [];

        let filtered = [...allProducts];

        // Apply each filter type
        Object.entries(selectedFilters).forEach(([filterId, values]) => {
            if (!values || values.length === 0) return;

            // For each filter value, keep products that match ANY of the values
            filtered = filtered.filter(product => {
                return values.some(value => {
                    switch (filterId) {
                        case 'categories':
                            return product.categories && product.categories.includes(value);

                        case 'brands':
                            // Check both direct brands and attributes
                            if (product.brands && product.brands.includes(value)) return true;
                            if (product.attributes) {
                                const brandAttr = product.attributes.find(attr => attr.key === 'brand');
                                if (brandAttr && brandAttr.value?.text && brandAttr.value.text.includes(value)) {
                                    return true;
                                }
                            }
                            return false;

                        case 'prices':
                            const price = product.priceInfo?.price ? parseFloat(product.priceInfo.price) : null;
                            if (price === null) return false;
                            const [min, max] = value.split('-').map(parseFloat);
                            return price >= min && price <= max;

                        case 'colors':
                            const productColors = [
                                ...(product.colorInfo?.colors || []),
                                ...(product.colorInfo?.colorFamilies || [])
                            ];
                            return productColors.some(color => color === value);

                        case 'sizes':
                            return product.sizes && product.sizes.includes(value);

                        case 'availability':
                            return product.availability === value;

                        default:
                            // Handle dynamic attribute filters
                            if (filterId.startsWith('attr_')) {
                                const attrKey = filterId.substring(5);
                                if (product.attributes) {
                                    const attr = product.attributes.find(a => a.key === attrKey);
                                    if (attr && attr.value?.text) {
                                        return attr.value.text.includes(value);
                                    }
                                }
                                return false;
                            }
                            return true;
                    }
                });
            });
        });

        return filtered;
    }, [allProducts, selectedFilters]);

    // OPTIMIZATION: Memoized filter simulation function
    const simulateFilter = useCallback((filterId, value, products) => {
        // Implementation of filter simulation logic
        // This now receives a pre-computed product list to avoid recalculation
        return products.filter(product => {
            switch (filterId) {
                case 'categories':
                    return product.categories && product.categories.includes(value);

                case 'brands':
                    if (product.brands && product.brands.includes(value)) return true;
                    if (product.attributes) {
                        const brandAttr = product.attributes.find(attr => attr.key === 'brand');
                        if (brandAttr && brandAttr.value?.text && brandAttr.value.text.includes(value)) {
                            return true;
                        }
                    }
                    return false;

                case 'prices':
                    const price = product.priceInfo?.price ? parseFloat(product.priceInfo.price) : null;
                    if (price === null) return false;
                    const [min, max] = value.split('-').map(parseFloat);
                    return price >= min && price <= max;

                case 'colors':
                    const productColors = [
                        ...(product.colorInfo?.colors || []),
                        ...(product.colorInfo?.colorFamilies || [])
                    ];
                    return productColors.some(color => color === value);

                case 'sizes':
                    return product.sizes && product.sizes.includes(value);

                case 'availability':
                    return product.availability === value;

                default:
                    if (filterId.startsWith('attr_')) {
                        const attrKey = filterId.substring(5);
                        if (product.attributes) {
                            const attr = product.attributes.find(a => a.key === attrKey);
                            if (attr && attr.value?.text) {
                                return attr.value.text.includes(value);
                            }
                        }
                        return false;
                    }
                    return true;
            }
        });
    }, []);

    // OPTIMIZATION: Memoize hasAvailableOptions function
    const hasAvailableOptions = useCallback((filterId, options) => {
        const currentSelection = selectedFilters[filterId] || [];

        // Early exit if all options already selected
        if (currentSelection.length >= options.length && options.length > 0) {
            return false;
        }

        // Only check up to 5 options to determine availability (performance optimization)
        // In real usage, if any option is available, that's enough to show the question
        const optionsToCheck = options.slice(0, Math.min(5, options.length));

        return optionsToCheck.some(option => {
            const value = option.value || option;
            // Skip already selected options
            if (currentSelection.includes(value)) return false;

            // Only simulate on first few products (performance optimization)
            const productSample = filteredProducts.slice(0, 50);
            const resultsWithFilter = simulateFilter(filterId, value, productSample);

            return resultsWithFilter.length > 0;
        });
    }, [filteredProducts, selectedFilters, simulateFilter]);

    // OPTIMIZATION: Memoize getSmartOptions with better performance
    const getSmartOptions = useCallback((filterId, options) => {
        const currentSelection = selectedFilters[filterId] || [];

        // Start by filtering out options that wouldn't return products
        // Use a smaller product set for initial filtering
        const productSample = filteredProducts.slice(0, 100);

        const validOptions = options.filter(option => {
            const value = option.value || option;

            // Skip already selected options
            if (currentSelection.includes(value)) return false;

            // Quick check with sample data
            const resultsWithFilter = simulateFilter(filterId, value, productSample);
            return resultsWithFilter.length > 0;
        });

        // For smaller option sets, do more precise count
        if (validOptions.length <= 10) {
            // Full evaluation on the options we have left
            const optionsWithCount = validOptions.map(option => {
                const value = option.value || option;
                const resultsWithFilter = simulateFilter(filterId, value, filteredProducts);

                return {
                    ...option,
                    resultCount: resultsWithFilter.length
                };
            });

            // Sort by product count (most to least)
            optionsWithCount.sort((a, b) => b.resultCount - a.resultCount);
            return optionsWithCount;
        }

        // For larger sets, avoid the expensive counting operation
        return validOptions.slice(0, 10);
    }, [filteredProducts, selectedFilters, simulateFilter]);

    // Function to fetch AI-generated filter suggestions
    const fetchAIFilterSuggestions = async (query) => {
        setIsLoadingAI(true);
        try {
            // OPTIMIZATION: Only extract essential data for API
            const categories = [...new Set(allProducts.flatMap(p => p.categories || []))];
            const brands = [...new Set(allProducts.flatMap(p => p.brands || []))];

            // Build a product context object with summary info
            const productContext = {
                total_products: allProducts.length,
                categories: categories.slice(0, 30), // Limit size
                brands: brands.slice(0, 30), // Limit size
                available_filters: filterConfigs.map(config => ({
                    id: config.id,
                    title: config.title,
                    option_count: config.options.length
                }))
            };

            // Add current filters to conversation history
            const updatedHistory = [
                ...conversationHistory,
                { role: "user", content: `I'm looking for: ${query}` }
            ];

            // API call
            const response = await getConversationalSearch(
                `For the search query "${query}", generate dynamic filter questions and suggestions to help users find products. Use the available filters to generate natural questions that sound conversational, not mechanical. Format response with: 1) General guidance 2) 3-5 follow-up questions 3) For each filter type, suggest a natural-sounding question to ask the user about that filter.`,
                updatedHistory,
                productContext
            );

            // Update conversation history
            setConversationHistory([
                ...updatedHistory,
                { role: "assistant", content: response.answer }
            ]);

            // Process the filter questions from the response
            if (response.follow_up_questions && response.follow_up_questions.length > 0) {
                setAiQuestions(response.follow_up_questions);

                // Extract question mappings
                if (response.suggested_products && response.suggested_products.length > 0) {
                    const newQuestionMap = {};
                    response.suggested_products.forEach(suggestion => {
                        if (suggestion.id && suggestion.reason) {
                            newQuestionMap[suggestion.id] = suggestion.reason;
                        }
                    });

                    // Update state with the new question mappings
                    setAiQuestionMap(newQuestionMap);
                }
            }
        } catch (error) {
            console.error('Error fetching AI filter suggestions:', error);
        } finally {
            setIsLoadingAI(false);
        }
    };

    // OPTIMIZATION: This useEffect now only runs when necessary inputs change
    useEffect(() => {
        if (!filterConfigs || !filterConfigs.length) {
            setQuestions([]);
            return;
        }

        // Generate questions from filter configs
        const newQuestions = filterConfigs.map(config => {
            // Use AI-generated question or fallback
            const questionText = aiQuestionMap[config.id] ||
                `Looking for specific ${config.title.toLowerCase()}?`;

            return {
                id: config.id,
                question: questionText,
                baseOptions: config.options,
                filterType: config.id
            };
        });

        setQuestions(newQuestions);
    }, [filterConfigs, aiQuestionMap]);

    // OPTIMIZATION: Only run this when questions or dependencies change
    const availableQuestions = useMemo(() => {
        return questions.map((question, index) => ({
            ...question,
            isAvailable: hasAvailableOptions(question.filterType, question.baseOptions),
            index
        })).filter(q => q.isAvailable);
    }, [questions, hasAvailableOptions]);

    // Find the next available question index - now memoized
    const findNextAvailableQuestion = useCallback((startIndex) => {
        const nextQuestion = availableQuestions.find(q => q.index > startIndex);
        return nextQuestion ? nextQuestion.index : -1;
    }, [availableQuestions]);

    // Handle option selection
    const handleOptionClick = useCallback((filterId, value) => {
        // Apply the filter
        onFilterChange(filterId, value, true);

        // Record this action in history
        setFilterHistory(prev => [...prev, {
            filterType: filterId,
            value,
            questionIndex: currentQuestionIndex
        }]);

        // Move to the next question
        const nextIndex = findNextAvailableQuestion(currentQuestionIndex);
        setCurrentQuestionIndex(nextIndex);
    }, [currentQuestionIndex, findNextAvailableQuestion, onFilterChange]);

    // Handle undo/back action
    const handleUndo = useCallback(() => {
        if (filterHistory.length === 0) return;

        // Get the last applied filter
        const lastFilter = filterHistory[filterHistory.length - 1];

        // Remove the filter
        onFilterChange(lastFilter.filterType, lastFilter.value, false);

        // Update history
        setFilterHistory(prev => prev.slice(0, -1));

        // Go back to the question where this filter was applied
        setCurrentQuestionIndex(lastFilter.questionIndex);
    }, [filterHistory, onFilterChange]);

    // Don't render if not visible
    if (!isVisible) {
        return null;
    }

    // If loading AI suggestions, show loading state
    if (isLoadingAI) {
        return (
            <Paper
                elevation={0}
                sx={{
                    mb: 3,
                    p: 2.5,
                    background: '#f5f9ff',
                    borderRadius: '8px',
                    position: 'relative',
                    border: '1px solid #e0e9f7',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    flexDirection: 'column',
                    minHeight: '150px'
                }}
            >
                <CircularProgress size={32} sx={{ mb: 2 }} />
                <Typography variant="body1">Generating smart filter suggestions...</Typography>
            </Paper>
        );
    }

    // Use standard filter workflow
    // Don't render if no more questions
    if (currentQuestionIndex === -1 || !questions.length) {
        return null;
    }

    // Get the current question
    const currentQuestion = questions[currentQuestionIndex];
    if (!currentQuestion) return null;

    // Check if question has available options
    const hasOptions = hasAvailableOptions(currentQuestion.filterType, currentQuestion.baseOptions);

    if (!hasOptions) {
        // Find next question with available options
        const nextIndex = findNextAvailableQuestion(currentQuestionIndex);

        // If there's no next question, don't render anything
        if (nextIndex === -1) return null;

        // Update index and don't render anything for this cycle
        setTimeout(() => setCurrentQuestionIndex(nextIndex), 0);
        return null;
    }

    // Get options for current question - compute this only when needed
    const options = getSmartOptions(currentQuestion.filterType, currentQuestion.baseOptions);

    return (
        <Paper
            elevation={0}
            sx={{
                mb: 3,
                p: 2.5,
                background: '#f5f9ff',
                borderRadius: '8px',
                position: 'relative',
                border: '1px solid #e0e9f7'
            }}
        >
            {/* Gemini Badge */}
            <Box sx={{
                position: 'absolute',
                left: 12,
                top: 12,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                width: 28,
                height: 28,
                borderRadius: '50%',
                backgroundColor: 'white',
                boxShadow: '0 1px 3px rgba(0,0,0,0.12)'
            }}>
                <GeminiIcon />
            </Box>

            {/* Question Text */}
            <Typography
                variant="h6"
                sx={{
                    fontWeight: 500,
                    mb: 2,
                    ml: 4
                }}
            >
                {currentQuestion.question}
            </Typography>

            {/* Options */}
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                {options.map((option, index) => {
                    const value = option.value || option;
                    const label = option.label || option;

                    return (
                        <Button
                            key={`${value}-${index}`}
                            variant="outlined"
                            color="primary"
                            size="small"
                            onClick={() => handleOptionClick(currentQuestion.filterType, value)}
                            sx={{
                                borderRadius: '16px',
                                px: 2,
                                textTransform: 'none',
                                borderColor: '#d0d7de',
                                color: '#333',
                                '&:hover': {
                                    borderColor: '#4285F4',
                                    backgroundColor: '#f0f7ff',
                                }
                            }}
                        >
                            {label}
                        </Button>
                    );
                })}
            </Box>

            {/* Bottom controls */}
            <Box sx={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'flex-start',
                mt: 2,
                flexWrap: 'wrap'
            }}>
                {/* Left side - Undo and filters */}
                <Box sx={{ maxWidth: '70%' }}>
                    {filterHistory.length > 0 && (
                        <>
                            <Button
                                variant="text"
                                color="primary"
                                size="small"
                                startIcon={<UndoIcon />}
                                onClick={handleUndo}
                                sx={{
                                    textTransform: 'none',
                                    mb: 1
                                }}
                            >
                                Undo last filter
                            </Button>

                            <Stack direction="row" spacing={1} sx={{ flexWrap: 'wrap', gap: 1 }}>
                                {filterHistory.map((filter, idx) => {
                                    // Find the filter label
                                    const filterConfig = questions.find(q => q.filterType === filter.filterType);
                                    if (!filterConfig) return null;

                                    const option = filterConfig.baseOptions.find(opt =>
                                        (opt.value || opt) === filter.value
                                    );
                                    const filterLabel = option ? (option.label || option) : filter.value;

                                    return (
                                        <Chip
                                            key={idx}
                                            label={filterLabel}
                                            size="small"
                                            color="primary"
                                            variant="outlined"
                                        />
                                    );
                                })}
                            </Stack>
                        </>
                    )}
                </Box>

                {/* Right side - Skip button */}
                <Button
                    variant="text"
                    size="small"
                    onClick={() => {
                        const nextIndex = findNextAvailableQuestion(currentQuestionIndex);
                        setCurrentQuestionIndex(nextIndex);
                    }}
                    sx={{
                        color: '#777',
                        textTransform: 'none'
                    }}
                >
                    Skip
                </Button>
            </Box>
        </Paper>
    );
};

// Export a memoized version of the component to prevent unnecessary re-renders
export default React.memo(AIFilterSuggestion);
