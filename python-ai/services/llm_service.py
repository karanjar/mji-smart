from langchain.chains import LLMChain
from langchain.prompts import PromptTemplate
from langchain_openai import ChatOpenAI
from langchain.cache import InMemoryCache
from langchain.globals import set_llm_cache
from typing import Optional
import asyncio
from dataclasses import dataclass

@dataclass
class SeverityResult:
    score: int
    confidence: float
    reason: str
    key_factors: list

class LLMService:
    def __init__(self, api_key: str, model: str = "gpt-3.5-turbo", use_cache: bool = True):
        self.llm = ChatOpenAI(
            api_key=api_key,
            model=model,
            temperature=0,
            max_tokens=100,
            timeout=10
        )
        
        if use_cache:
            set_llm_cache(InMemoryCache())
        
        # Create severity scoring chain
        severity_template = PromptTemplate(
            input_variables=["description", "category", "image_analysis"],
            template="""
            You are an infrastructure severity analyst for a smart city platform. Analyze the report and assign a severity score 1-5.

            Report Category: {category}
            User Description: {description}
            Image Analysis: {image_analysis}

            Severity Scale:
            1: Minor - Cosmetic issue, no immediate danger
            2: Moderate - Noticeable problem, requires attention soon
            3: Significant - Actively causing disruption, needs attention today
            4: Severe - Dangerous situation, requires immediate response
            5: Critical - Emergency, risk to life or property

            Consider:
            - Safety risk to citizens
            - Impact on infrastructure
            - Urgency of repair
            - Scale of the problem

            Respond with JSON:
            {{
                "score": integer 1-5,
                "confidence": float 0-1,
                "reason": "brief justification",
                "key_factors": ["factor1", "factor2"]
            }}
            """
        )
        
        self.severity_chain = LLMChain(llm=self.llm, prompt=severity_template)
    
    async def score_severity(
        self,
        description: str,
        category: str,
        image_analysis: dict
    ) -> SeverityResult:
        """Score severity using LLM"""
        try:
            # Format image analysis for prompt
            image_summary = f"Primary issue: {image_analysis.get('primary_issue', 'unknown')}, Quality: {image_analysis.get('quality', 'unknown')}"
            
            # Run chain
            result = await self.severity_chain.apredict(
                description=description,
                category=category,
                image_analysis=image_summary
            )
            
            # Parse JSON response
            import json
            data = json.loads(result)
            
            return SeverityResult(
                score=data["score"],
                confidence=data["confidence"],
                reason=data["reason"],
                key_factors=data["key_factors"]
            )
            
        except Exception as e:
            # Fallback to rule-based scoring
            return self._rule_based_scoring(description, category)
    
    def _rule_based_scoring(self, description: str, category: str) -> SeverityResult:
        """Fallback rule-based severity scoring"""
        score = 3  # Default moderate
        confidence = 0.6
        
        # Keywords that increase severity
        high_priority_keywords = ["emergency", "danger", "injury", "blocked", "flooding", "burst"]
        low_priority_keywords = ["minor", "small", "cosmetic", "slight"]
        
        desc_lower = description.lower()
        
        if any(kw in desc_lower for kw in high_priority_keywords):
            score = 4
            confidence = 0.7
        if any(kw in desc_lower for kw in low_priority_keywords):
            score = 2
            confidence = 0.7
        
        # Category-specific adjustments
        if category == "burst_pipe":
            score = max(score, 4)
        elif category == "flooding":
            score = max(score, 4)
        elif category == "illegal_dumping":
            score = min(score, 3)
        
        return SeverityResult(
            score=score,
            confidence=confidence,
            reason="Rule-based fallback scoring",
            key_factors=["keyword_matching"]
        )