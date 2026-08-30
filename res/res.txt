package generated

import (
	"context"
	"fmt"
)

// ExecutionPipeline est généré automatiquement depuis le graphe
func ExecutionPipeline(ctx context.Context) error {
	// Statut HTTP configuré
	fmt.Println("Réponse envoyée avec succès", results)
	// --- Début du pipeline (rootNode_6pdmo) ---
	// Modèle cible: voiture
	query := db.Model(&SelectNode{}).Select("*")
	query = query.Where("1 = 1")
	var results []interface{}
	if err := query.Find(&results).Error; err != nil {
		return fmt.Errorf("échec de la requête: %w", err)
	}
	fmt.Println("Réponse envoyée avec succès", results)
	return nil
}
